// FeedbackPopup — rich feedback UX for vote buttons
import { waitForPreact, copyToClipboard } from "./utils.js";
import { getReviewMeta } from "./reviewMeta.mjs";

// module-level cache — fetched once per session
let _impactStats = null;
let _impactStatsFetching = false;
const _impactStatsCallbacks = [];

// track last stored feedback id per visibilityKey so we can retract on vote-switch
const _feedbackIds = {};

function storeFeedbackId(key, id) {
  _feedbackIds[key] = id;
}

function retractStoredFeedback(key) {
  const id = _feedbackIds[key];
  if (!id) return;
  delete _feedbackIds[key];
  fetch(`/api/v1/feedback/${id}/retract`, { method: "PATCH" }).catch(() => {});
}

export function fetchImpactStats(reviewID, onReady) {
  if (_impactStats) {
    onReady(_impactStats);
    return;
  }
  _impactStatsCallbacks.push(onReady);
  if (_impactStatsFetching) return;
  _impactStatsFetching = true;
  const url = reviewID
    ? `/api/v1/feedback/impact-stats?review_id=${reviewID}`
    : `/api/v1/feedback/impact-stats`;
  fetch(url)
    .then((r) => r.json())
    .then((data) => {
      _impactStats = [
        {
          label: "Total Reviews",
          value: data.total_reviews,
          tooltip: "Total completed reviews",
        },
        {
          label: "Issues Found",
          value: data.issues_found,
          tooltip: "Sum of all severity issues",
        },
        {
          label: "Bugs Caught Pre-Prod",
          value: data.bugs_caught,
          tooltip: "Critical + Error issues",
        },
        {
          label: "Critical",
          value: data.critical,
          tooltip: "Critical severity issues",
        },
        {
          label: "Errors",
          value: data.errors,
          tooltip: "Error severity issues",
        },
        {
          label: "Warnings",
          value: data.warnings,
          tooltip: "Warning severity issues",
        },
        { label: "Info", value: data.info, tooltip: "Info severity comments" },
      ];
      _impactStatsCallbacks.splice(0).forEach((cb) => cb(_impactStats));
    })
    .catch(() => {
      _impactStatsFetching = false;
      _impactStatsCallbacks.length = 0;
    });
}

const DOWN_TAGS = [
  "False positive",
  "Wrong severity",
  "Missed something",
  "Hard to act on",
];

export const LINKEDIN_TEXT = `🚀 Shipping with confidence — here's my code review impact since Jan 2025:

✅ 47 reviews completed
🐛 189 bugs caught before production
⚡ 8s average first comment time
🔴 23 critical issues found
🟠 166 errors caught
🟡 123 warnings flagged

Using git-lrc to AI-review every commit before it lands.

⭐ Star it if you find it useful: https://github.com/HexmosTech/git-lrc

#CodeReview #DevOps #SoftwareEngineering #AI`;

export async function createFeedbackPopup() {
  const { html, useState, useRef, useEffect } = await waitForPreact();

  return function FeedbackPopup({
    type,
    vote,
    onVote,
    visibilityKey,
    commentContent,
    codeExcerpt,
    filePath,
    severity,
    sourceType,
  }) {
    const wrapperRef = useRef(null);
    const popupRef = useRef(null);

    const [popupVisible, setPopupVisible] = useState(false);
    const [popupOpacity, setPopupOpacity] = useState(0);
    const [popupShift, setPopupShift] = useState(-6);
    const [popupMode, setPopupMode] = useState(null); // 'hover'|'click'|'submitted'
    const [feedbackText, setFeedbackText] = useState("");
    const [selectedTags, setSelectedTags] = useState(new Set());
    const [statsExpanded, setStatsExpanded] = useState(false);
    const [linkedinOpen, setLinkedinOpen] = useState(false);
    const [linkedinOpacity, setLinkedinOpacity] = useState(0);
    const [linkedinText, setLinkedinText] = useState(LINKEDIN_TEXT);
    const [snackbar, setSnackbar] = useState(false);
    const [popupPos, setPopupPos] = useState({ top: 0, left: 0 });
    const [tentativeDown, setTentativeDown] = useState(false); // red but not yet submitted
    const [impactStats, setImpactStats] = useState(_impactStats);
    const [submitting, setSubmitting] = useState(false);
    const [submitError, setSubmitError] = useState(false);

    const autoTimer = useRef(null);
    const hoverTimer = useRef(null);
    const snackTimer = useRef(null);
    const closeLIRef = useRef(null);

    const isActive = vote === type || (type === "down" && tentativeDown);

    // ── cleanup on unmount ────────────────────────────────────────────────
    useEffect(
      () => () => {
        [autoTimer, hoverTimer, snackTimer].forEach(
          (r) => r.current && clearTimeout(r.current),
        );
      },
      [],
    );

    // ── clear tentativeDown when sibling vote is committed ────────────────
    useEffect(() => {
      if (type === "down" && vote === "up" && tentativeDown) {
        setTentativeDown(false);
        setPopupOpacity(0);
        setPopupShift(-4);
        setTimeout(() => {
          setPopupVisible(false);
          setPopupMode(null);
        }, 280);
      }
    }, [vote]);

    // ── ESC closes linkedin overlay ───────────────────────────────────────
    useEffect(() => {
      if (!linkedinOpen) return;
      const handler = (e) => {
        if (e.key === "Escape") closeLIRef.current?.();
      };
      document.addEventListener("keydown", handler);
      return () => document.removeEventListener("keydown", handler);
    }, [linkedinOpen]);

    // ── helpers ───────────────────────────────────────────────────────────
    const buttonColor = () => {
      if (isActive && type === "up") return "#22c55e";
      if (isActive && type === "down") return "#ef4444";
      return "rgba(255,255,255,0.65)";
    };
    const buttonBorder = () => {
      if (isActive && type === "up") return "1px solid #22c55e";
      if (isActive && type === "down") return "1px solid #ef4444";
      return "1px solid rgba(255,255,255,0.18)";
    };
    const buttonBg = () => {
      if (isActive && type === "up") return "rgba(34,197,94,0.15)";
      if (isActive && type === "down") return "rgba(239,68,68,0.15)";
      return "rgba(255,255,255,0.07)";
    };

    const popupWidth = () => 420;

    const show = (mode) => {
      if (!popupVisible) {
        // First show: render off-screen invisible so we can measure height
        setPopupPos({ top: -1000, left: -1000 });
        setPopupOpacity(0);
        setPopupShift(-6);
      }
      setPopupVisible(true);
      setPopupMode(mode);
    };

    // After popup renders, measure its actual height and position it correctly
    useEffect(() => {
      if (!popupVisible || !popupRef.current || !wrapperRef.current) return;
      const r = wrapperRef.current.getBoundingClientRect();
      const h = popupRef.current.offsetHeight;
      if (h === 0) return;
      const w = popupWidth();
      const left = Math.max(
        8,
        Math.min(r.right - w, window.innerWidth - w - 8),
      );
      const belowTop = r.bottom + 8;
      const top =
        belowTop + h > window.innerHeight
          ? Math.max(8, r.top - h - 8)
          : belowTop;
      setPopupPos({ top, left });
      if (popupOpacity === 0) {
        requestAnimationFrame(() =>
          requestAnimationFrame(() => {
            setPopupOpacity(1);
            setPopupShift(0);
          }),
        );
      }
    }, [popupVisible, popupMode]);

    const hide = () => {
      setPopupOpacity(0);
      setPopupShift(-4);
      setTentativeDown(false);
      setTimeout(() => {
        setPopupVisible(false);
        setPopupMode(null);
        setStatsExpanded(false);
      }, 280);
    };

    const clearAuto = () => {
      if (autoTimer.current) {
        clearTimeout(autoTimer.current);
        autoTimer.current = null;
      }
    };
    const startAuto = (ms = 5000) => {
      clearAuto();
      autoTimer.current = setTimeout(hide, ms);
    };

    const scheduleHoverClose = () => {
      hoverTimer.current = setTimeout(() => {
        if (popupMode === "hover") hide();
      }, 80);
    };
    const cancelHoverClose = () => {
      if (hoverTimer.current) {
        clearTimeout(hoverTimer.current);
        hoverTimer.current = null;
      }
    };

    // ── api helper ────────────────────────────────────────────────────────
    const postFeedback = (extra = {}) => {
      try {
        const { reviewID } = getReviewMeta();
        const body = {
          vote_type: type,
          source_type: sourceType || "comment",
          tags: [...selectedTags],
        };
        if (reviewID) body.review_id = Number(reviewID) || undefined;
        if (commentContent) body.comment_content = commentContent;
        if (filePath) body.file_path = filePath;
        if (severity) body.severity = severity;
        Object.assign(body, extra);
        fetch("/api/v1/feedback", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        })
          .then((r) => r.json())
          .then((data) => {
            if (data?.id) storeFeedbackId(visibilityKey, data.id);
          })
          .catch(() => {});
      } catch {}
    };

    // ── button events ─────────────────────────────────────────────────────
    const handleClick = (e) => {
      e.stopPropagation();
      if (type === "up") {
        if (vote === "up") {
          retractStoredFeedback(visibilityKey);
          if (onVote) onVote(visibilityKey, null);
          if (popupVisible) hide();
          return;
        }
        // switching from downvote — retract it
        if (vote === "down") retractStoredFeedback(visibilityKey);
        const { reviewID: rid } = getReviewMeta();
        fetchImpactStats(rid, (data) => setImpactStats(data));
        if (onVote) onVote(visibilityKey, "up");
        postFeedback();
        cancelHoverClose();
        show("click");
        startAuto();
      } else {
        if (vote === "down") {
          retractStoredFeedback(visibilityKey);
          if (onVote) onVote(visibilityKey, null);
          if (popupVisible) hide();
          return;
        }
        // switching from upvote — retract it and clear parent vote
        if (vote === "up") {
          retractStoredFeedback(visibilityKey);
          if (onVote) onVote(visibilityKey, null);
        }
        setTentativeDown(true);
        cancelHoverClose();
        show("click");
        startAuto();
      }
    };

    const handleMouseEnter = () => {
      cancelHoverClose();
      if (popupMode === "click" || popupMode === "submitted") return;
      if (type === "up" && !popupVisible) {
        const { reviewID } = getReviewMeta();
        fetchImpactStats(reviewID, (data) => setImpactStats(data));
        show("hover");
      }
    };

    const handleMouseLeave = () => {
      if (popupMode === "click" || popupMode === "submitted") return;
      scheduleHoverClose();
    };

    // ── popup events ──────────────────────────────────────────────────────
    const onPopupEnter = () => {
      cancelHoverClose();
      clearAuto();
    };

    const onPopupLeave = () => {
      if (popupMode === "click" || popupMode === "submitted") {
        hide();
      } else {
        scheduleHoverClose();
      }
    };

    // ── form ──────────────────────────────────────────────────────────────
    const handleSubmit = async (e) => {
      e.stopPropagation();
      clearAuto();
      setSubmitError(false);
      setSubmitting(true);
      try {
        const { reviewID } = getReviewMeta();
        const body = {
          vote_type: type,
          source_type: sourceType || "comment",
          tags: [...selectedTags],
          ...(reviewID && { review_id: Number(reviewID) }),
          ...(commentContent && { comment_content: commentContent }),
          ...(filePath && { file_path: filePath }),
          ...(severity && { severity }),
          ...(feedbackText && { feedback_text: feedbackText }),
          ...(codeExcerpt && { code_excerpt: codeExcerpt }),
        };
        const res = await fetch("/api/v1/feedback", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        if (!res.ok) {
          setSubmitting(false);
          setSubmitError(true);
          startAuto(6000);
          return;
        }
        const data = await res.json().catch(() => ({}));
        if (data?.id) storeFeedbackId(visibilityKey, data.id);
      } catch {
        setSubmitting(false);
        setSubmitError(true);
        startAuto(6000);
        return;
      }
      setSubmitting(false);
      if (type === "down") {
        if (onVote) onVote(visibilityKey, "down");
        setTentativeDown(false);
      }
      setPopupMode("submitted");
    };

    const toggleTag = (tag) => {
      setSelectedTags((prev) => {
        const next = new Set(prev);
        next.has(tag) ? next.delete(tag) : next.add(tag);
        return next;
      });
    };

    // ── linkedin overlay ──────────────────────────────────────────────────
    const openLinkedin = () => {
      setLinkedinOpen(true);
      setLinkedinOpacity(0);
      requestAnimationFrame(() =>
        requestAnimationFrame(() => setLinkedinOpacity(1)),
      );
    };

    const closeLinkedin = () => {
      setLinkedinOpacity(0);
      setTimeout(() => setLinkedinOpen(false), 200);
    };
    closeLIRef.current = closeLinkedin;

    const handleCopyLinkedin = async (e) => {
      e.stopPropagation();
      try {
        await copyToClipboard(linkedinText);
        setSnackbar(true);
        if (snackTimer.current) clearTimeout(snackTimer.current);
        snackTimer.current = setTimeout(() => setSnackbar(false), 2200);
      } catch {}
    };

    // ── sub-renders ───────────────────────────────────────────────────────
    const ImpactLink = () =>
      statsExpanded
        ? html`<div
            style="font-size:12px;color:#4a5a6a;padding:4px 0;user-select:none;"
          >
            ✨ Want to see your impact stats?
          </div>`
        : html`
            <div
              style="display:flex;align-items:center;gap:5px;color:#7aadff;cursor:pointer;font-size:12px;font-weight:500;padding:4px 0;user-select:none;transition:color 0.15s;"
              onMouseEnter=${() => setStatsExpanded(true)}
            >
              ✨ Want to see your impact stats?
              <svg
                width="11"
                height="11"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
              >
                <path d="M9 18l6-6-6-6" />
              </svg>
            </div>
          `;

    const StatsGrid = () => {
      const stats = impactStats;
      if (!stats)
        return html`<div style="font-size:12px;color:#4a5a6a;padding:8px 0;">
          Loading stats…
        </div>`;
      return html`
        <div style="margin-top:10px;">
          <div
            style="display:grid;grid-template-columns:repeat(4,1fr);gap:5px;margin-bottom:10px;"
          >
            ${stats.map(
              (s) => html`
                <div
                  title=${s.tooltip}
                  style="background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.08);border-radius:8px;padding:8px 6px;text-align:center;cursor:default;"
                >
                  <div
                    style="font-size:17px;font-weight:700;color:#7aadff;line-height:1.2;"
                  >
                    ${s.value}
                  </div>
                  <div
                    style="font-size:10px;color:#6a88aa;margin-top:3px;line-height:1.3;"
                  >
                    ${s.label}
                  </div>
                </div>
              `,
            )}
          </div>
          <div
            style="border-top:1px solid rgba(255,255,255,0.07);padding-top:9px;"
          >
            <div
              style="font-size:12px;font-weight:600;color:#7aadff;cursor:pointer;display:flex;align-items:center;gap:6px;transition:color 0.15s;"
              onMouseEnter=${(e) => {
                e.currentTarget.style.color = "#a8caff";
                openLinkedin();
              }}
              onMouseLeave=${(e) => {
                e.currentTarget.style.color = "#7aadff";
              }}
            >
              <span
                >✦ Stand out by showing your impact stats to your peers</span
              >
              <svg
                width="12"
                height="12"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
              >
                <path d="M9 18l6-6-6-6" />
              </svg>
            </div>
          </div>
        </div>
      `;
    };

    const popupBase = `position:fixed;top:${popupPos.top}px;left:${popupPos.left}px;z-index:20000;background:#151f2e;border:1px solid rgba(99,130,180,0.22);border-radius:12px;box-shadow:0 8px 32px rgba(0,0,0,0.5),0 2px 8px rgba(0,0,0,0.3);padding:14px 16px;width:${popupWidth()}px;opacity:${popupOpacity};transform:translateY(${popupShift}px);transition:opacity 0.28s ease,transform 0.28s ease;font-size:13px;color:#c9d5e8;`;

    const inputStyle =
      "width:100%;box-sizing:border-box;background:rgba(255,255,255,0.06);border:1px solid rgba(255,255,255,0.12);border-radius:6px;color:#c9d5e8;font-size:12px;padding:7px 9px;resize:vertical;min-height:58px;font-family:inherit;outline:none;";
    const submitStyle =
      "margin-top:8px;padding:5px 14px;background:#2d5be3;border:none;border-radius:6px;color:white;font-size:12px;font-weight:600;cursor:pointer;transition:background 0.15s;";
    const headingStyle =
      "font-weight:600;color:#e8f0ff;margin-bottom:10px;font-size:13px;line-height:1.4;";
    const subStyle = "color:#8899bb;font-size:12px;margin-bottom:6px;";

    return html`
      <div ref=${wrapperRef} style="position:relative;display:inline-block;">
        <button
          title=${type === "up" ? "Helpful" : "Not helpful"}
          onClick=${handleClick}
          onMouseEnter=${handleMouseEnter}
          onMouseLeave=${handleMouseLeave}
          style="position:static;display:flex;align-items:center;justify-content:center;width:28px;height:28px;border-radius:6px;cursor:pointer;transition:all 0.15s ease;flex-shrink:0;color:${buttonColor()};border:${buttonBorder()};background:${buttonBg()};"
        >
          ${type === "up"
            ? html`<svg
                width="13"
                height="13"
                viewBox="0 0 24 24"
                fill="currentColor"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  d="M14 9V5a3 3 0 00-3-3l-4 9v11h11.28a2 2 0 002-1.7l1.38-9a2 2 0 00-2-2.3H14z"
                />
                <path d="M7 22H4a2 2 0 01-2-2v-7a2 2 0 012-2h3" />
              </svg>`
            : html`<svg
                width="13"
                height="13"
                viewBox="0 0 24 24"
                fill="currentColor"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  d="M10 15v4a3 3 0 003 3l4-9V2H5.72a2 2 0 00-2 1.7l-1.38 9a2 2 0 002 2.3H10z"
                />
                <path
                  d="M17 2h2.67A2.31 2.31 0 0122 4v7a2.31 2.31 0 01-2.33 2H17"
                />
              </svg>`}
        </button>

        ${popupVisible &&
        html`
          <div
            ref=${popupRef}
            style=${popupBase}
            onMouseEnter=${onPopupEnter}
            onMouseLeave=${onPopupLeave}
            onClick=${(e) => e.stopPropagation()}
          >
            ${popupMode === "hover" &&
            type === "up" &&
            html`
              <div>
                <${ImpactLink} />
                ${statsExpanded && html`<${StatsGrid} />`}
              </div>
            `}
            ${popupMode === "click" &&
            type === "up" &&
            html`
              <div>
                <div style=${headingStyle}>👍 Thanks for your feedback!</div>
                <div style=${subStyle}>
                  What did you like about this review comment?
                </div>
                <textarea
                  placeholder="Share your thoughts..."
                  value=${feedbackText}
                  onInput=${(e) => setFeedbackText(e.target.value)}
                  style=${inputStyle}
                  onClick=${(e) => e.stopPropagation()}
                ></textarea>
                <div
                  style="font-size:10px;color:#4a5a6a;margin-top:6px;line-height:1.4;"
                >
                  This comment and the code block will be sent to Hexmos to
                  continue improving quality.
                </div>
                ${submitError && html`<div style="font-size:11px;color:#ef4444;margin-top:6px;">Failed to send — please try again.</div>`}
                <button
                  onClick=${handleSubmit}
                  disabled=${submitting}
                  style="${submitStyle}margin-top:8px;opacity:${submitting ? '0.6' : '1'};"
                >
                  ${submitting ? "Sending…" : "Submit More"}
                </button>
                <div
                  style="margin-top:12px;padding-top:10px;border-top:1px solid rgba(255,255,255,0.07);"
                >
                  <${ImpactLink} />
                  ${statsExpanded && html`<${StatsGrid} />`}
                </div>
              </div>
            `}
            ${popupMode === "submitted" &&
            type === "up" &&
            html`
              <div
                style="min-height:220px;display:flex;flex-direction:column;justify-content:space-between;"
              >
                <div style=${headingStyle}>
                  🎉 Thanks for your detailed feedback!
                </div>
                <div
                  style="margin-top:8px;padding-top:8px;border-top:1px solid rgba(255,255,255,0.07);"
                >
                  <${ImpactLink} />
                  ${statsExpanded && html`<${StatsGrid} />`}
                </div>
              </div>
            `}
            ${popupMode === "click" &&
            type === "down" &&
            html`
              <div>
                <div style=${headingStyle}>
                  👎 We're sorry it didn't meet your expectations!
                </div>
                <div style="margin-bottom:8px;">
                  <div
                    style="color:#8899bb;font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:0.05em;margin-bottom:6px;"
                  >
                    What went wrong?
                  </div>
                  <div style="display:flex;flex-wrap:wrap;gap:5px;">
                    ${DOWN_TAGS.map(
                      (tag) => html`
                        <button
                          onClick=${(e) => {
                            e.stopPropagation();
                            toggleTag(tag);
                          }}
                          style="padding:3px 10px;border-radius:20px;font-size:11px;cursor:pointer;transition:all 0.15s;border:1px solid ${selectedTags.has(
                            tag,
                          )
                            ? "#ef4444"
                            : "rgba(255,255,255,0.15)"};background:${selectedTags.has(
                            tag,
                          )
                            ? "rgba(239,68,68,0.18)"
                            : "rgba(255,255,255,0.03)"};color:${selectedTags.has(
                            tag,
                          )
                            ? "#ef4444"
                            : "#8899bb"};"
                        >
                          ${tag}
                        </button>
                      `,
                    )}
                  </div>
                </div>
                <textarea
                  placeholder="Tell us more... (optional)"
                  value=${feedbackText}
                  onInput=${(e) => setFeedbackText(e.target.value)}
                  style=${inputStyle}
                  onClick=${(e) => e.stopPropagation()}
                ></textarea>
                <div
                  style="font-size:10px;color:#4a5a6a;margin-top:6px;line-height:1.4;"
                >
                  This comment and the code block will be sent to Hexmos. A
                  human will review it to understand and ship a fix.
                </div>
                ${submitError && html`<div style="font-size:11px;color:#ef4444;margin-top:6px;">Failed to send — please try again.</div>`}
                <button
                  onClick=${handleSubmit}
                  disabled=${submitting}
                  style="${submitStyle}margin-top:8px;opacity:${submitting ? '0.6' : '1'};"
                >
                  ${submitting ? "Sending…" : "Submit"}
                </button>
              </div>
            `}
            ${popupMode === "submitted" &&
            type === "down" &&
            html`
              <div>
                <div style=${headingStyle}>
                  🙏 Thanks — we'll work on making it better!
                </div>
              </div>
            `}
          </div>
        `}
        ${linkedinOpen &&
        html`
          <div
            style="position:fixed;inset:0;z-index:30000;background:rgba(0,0,0,0.72);backdrop-filter:blur(6px);display:flex;align-items:center;justify-content:center;opacity:${linkedinOpacity};transition:opacity 0.2s ease;"
            onClick=${(e) => {
              if (e.target === e.currentTarget) closeLinkedin();
            }}
          >
            <div
              style="background:#151f2e;border:1px solid rgba(99,130,180,0.22);border-radius:16px;padding:32px;max-width:600px;width:calc(100vw - 48px);max-height:calc(100vh - 80px);overflow-y:auto;position:relative;box-shadow:0 24px 64px rgba(0,0,0,0.55);"
              onClick=${(e) => e.stopPropagation()}
            >
              <button
                onClick=${closeLinkedin}
                title="Close (Esc)"
                style="position:absolute;top:16px;right:16px;background:rgba(255,255,255,0.07);border:1px solid rgba(255,255,255,0.12);border-radius:6px;color:#8899bb;cursor:pointer;padding:4px 9px;font-size:14px;line-height:1;transition:background 0.15s;"
              >
                ✕
              </button>
              <div
                style="font-weight:700;font-size:17px;color:#e8f0ff;margin-bottom:4px;"
              >
                Share your impact with your peers
              </div>
              <div style="font-size:12px;color:#5a7aaa;margin-bottom:18px;">
                Edit and post on LinkedIn to showcase your engineering impact 🚀
              </div>
              <textarea
                value=${linkedinText}
                onInput=${(e) => setLinkedinText(e.target.value)}
                style="width:100%;box-sizing:border-box;background:rgba(255,255,255,0.04);border:1px solid rgba(255,255,255,0.1);border-radius:8px;color:#c9d5e8;font-size:13px;line-height:1.65;padding:14px 16px;min-height:300px;resize:vertical;font-family:inherit;outline:none;"
                onClick=${(e) => e.stopPropagation()}
              ></textarea>
              <button
                onClick=${handleCopyLinkedin}
                style="margin-top:16px;padding:9px 22px;background:${snackbar
                  ? "#22c55e"
                  : "#2d5be3"};border:none;border-radius:8px;color:white;font-size:13px;font-weight:600;cursor:pointer;transition:background 0.2s;display:flex;align-items:center;gap:8px;"
              >
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <rect x="9" y="9" width="13" height="13" rx="2" />
                  <path
                    d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"
                  />
                </svg>
                ${snackbar ? "✓ Copied!" : "Copy to clipboard"}
              </button>
            </div>
          </div>
        `}
        ${snackbar &&
        !linkedinOpen &&
        html`
          <div
            style="position:fixed;bottom:24px;left:50%;transform:translateX(-50%);background:#1e3a2f;border:1px solid #22c55e;border-radius:8px;padding:8px 20px;color:#4ade80;font-size:13px;font-weight:500;z-index:9999;box-shadow:0 4px 16px rgba(0,0,0,0.4);pointer-events:none;animation:fadeInUp 0.3s ease;"
          >
            ✓ Copied to clipboard!
          </div>
        `}
      </div>
    `;
  };
}

let FeedbackPopupComponent = null;
export async function getFeedbackPopup() {
  if (!FeedbackPopupComponent) {
    FeedbackPopupComponent = await createFeedbackPopup();
  }
  return FeedbackPopupComponent;
}
// add a freaking comment here
//
// i will drop the prod database
