// LiveReview App - Main Entry Point
// Fetches data from /api/review and updates reactively

import { waitForPreact, filePathToId, transformEvent, getBadgeClass, formatIssueForCopy, getCommentVisibilityKey } from './components/utils.js';
import { buildIssueCategoryGroups, buildIssueFacetOptions, buildIssueFilterUniverse, countIssuesByFilters, createDefaultIssueFilters, DEFAULT_SEVERITIES, getCommentFilterValue, getIssueFilterSummary, matchesIssueFilters, resetIssueFilters, toggleIssueFilterValue } from './components/issue_filter_state.mjs';
import { appendStreamedCommentsToFiles, buildEventsURL, extractExternalCommentsFromEvents, extractNewEvents, inferReviewStatusFromEvents } from './components/review_stream_state.mjs';
import { getHeader } from './components/Header.js';
import { getSidebar } from './components/Sidebar.js';
import { getSummary } from './components/Summary.js';
import { getStats } from './components/Stats.js';
import { getPrecommitBar } from './components/PrecommitBar.js';
import { getFileBlock } from './components/FileBlock.js';
import { getEventLog } from './components/EventLog.js';
import { getIssueFilterBar } from './components/IssueFilterBar.js';
import { getSendToAgentInfo } from './components/SendToAgentButton.js';
import { renderHandoffConfetti } from './components/handoffConfetti.js';
import { getToolbar } from './components/Toolbar.js';
import { getCommentNav } from './components/CommentNav.js';
import { UsageBanner } from './components/UsageBanner.js';
import { renderIcon } from './components/icons.js';
import { getSummarySlideshow } from './components/SummarySlideshow/SummarySlideshow.js';
import { evaluateSummarySlidesEligibility } from './components/SummarySlideshow/slideshowParser.js';
import { buildPerformanceSnapshot, getFirstRenderTime, getLoadingActivityMessage, getPerformanceNow, recordFirstRenderTime } from './components/review_performance_state.mjs';
import { shouldShowAllClear } from './components/review_outcome_state.mjs';
import { setReviewMeta } from './components/reviewMeta.mjs';

let domReadyStartMs = null;

// Convert API response to UI data format
// Backend uses snake_case JSON keys (file_path, old_start_line, etc.)

// Helper: count actual comments from files array
function countCommentsFromFiles(files) {
    if (!files) return 0;
    return files.reduce((total, file) => {
        const comments = file.comments || file.Comments || [];
        return total + comments.length;
    }, 0);
}

// Build a severity + type breakdown for the handoff modal so the user can see the
// scope of what's being auto-fixed (e.g. "6 issues — 2 Critical, 3 Warning, 1 Info").
function buildHandoffImpactSummary(files) {
    const bySeverity = Object.fromEntries(DEFAULT_SEVERITIES.map((severity) => [severity, 0]));
    const typeCounts = new Map();
    let total = 0;
    (files || []).forEach((file) => {
        (file.comments || file.Comments || []).forEach((comment) => {
            total += 1;
            const severity = getCommentFilterValue(comment, 'severity');
            bySeverity[severity] = (bySeverity[severity] || 0) + 1;

            const type = getCommentFilterValue(comment, 'type');
            if (type) {
                typeCounts.set(type, (typeCounts.get(type) || 0) + 1);
            }
        });
    });
    const byType = [...typeCounts.entries()]
        .map(([label, count]) => ({ label, count }))
        .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));
    return { total, bySeverity, byType };
}

function convertFilesToUIFormat(files) {
    if (!files) return [];
    
    return files.map(file => {
        // Handle snake_case from backend
        const filePath = file.file_path || file.filePath || file.FilePath || '';
        // Use same ID generation as filePathToId in utils.js
        const fileId = 'file_' + filePath.replace(/[^a-zA-Z0-9]/g, '_');
        const comments = file.comments || file.Comments || [];
        const hunks = file.hunks || file.Hunks || [];

        const toLineNumber = (comment) => {
            const raw = comment.line ?? comment.Line ?? comment.line_number ?? comment.lineNumber ?? comment.LineNumber;
            const parsed = Number(raw);
            return Number.isFinite(parsed) ? parsed : 0;
        };
        
        // Build comment lookup by line
        const commentsByLine = {};
        comments.forEach(comment => {
            const line = toLineNumber(comment);
            if (line <= 0) return;
            if (!commentsByLine[line]) {
                commentsByLine[line] = [];
            }
            commentsByLine[line].push({
                Severity: (comment.severity || comment.Severity || 'info').toUpperCase(),
                Confidence: comment.confidence || comment.Confidence || '',
                Type: comment.type || comment.Type || '',
                BadgeClass: getBadgeClass(comment.severity || comment.Severity || 'info'),
                Category: comment.category || comment.Category || '',
                Subcategory: comment.subcategory || comment.Subcategory || '',
                Content: comment.content || comment.Content || '',
                HasCategory: !!(comment.category || comment.Category),
                Line: line,
                FilePath: filePath
            });
        });

        const takeCommentsForLine = (lineNumber) => {
            if (!lineNumber || lineNumber <= 0) return [];
            const bucket = commentsByLine[lineNumber];
            if (!bucket || bucket.length === 0) return [];
            const pending = bucket;
            commentsByLine[lineNumber] = [];
            return pending;
        };
        
        // Process hunks
        const processedHunks = hunks.map(hunk => {
            // Handle snake_case keys
            const oldStartLine = hunk.old_start_line || hunk.oldStartLine || hunk.OldStartLine || 1;
            const oldLineCount = hunk.old_line_count || hunk.oldLineCount || hunk.OldLineCount || 0;
            const newStartLine = hunk.new_start_line || hunk.newStartLine || hunk.NewStartLine || 1;
            const newLineCount = hunk.new_line_count || hunk.newLineCount || hunk.NewLineCount || 0;
            const header = hunk.header || hunk.Header || 
                `@@ -${oldStartLine},${oldLineCount} +${newStartLine},${newLineCount} @@`;
            
            // If hunk already has Lines array (pre-processed), use it
            if (hunk.Lines) {
                // Merge comments into existing lines
                const lines = hunk.Lines.map(line => {
                    const newNum = parseInt(line.NewNum, 10) || 0;
                    const oldNum = parseInt(line.OldNum, 10) || 0;
                    let lineComments = takeCommentsForLine(newNum);
                    if (lineComments.length === 0) {
                        lineComments = takeCommentsForLine(oldNum);
                    }
                    if (lineComments.length > 0) {
                        return {
                            ...line,
                            IsComment: true,
                            Comments: lineComments
                        };
                    }
                    return line;
                });
                return { Header: header, Lines: lines };
            }
            
            // Parse hunk content into lines
            const content = hunk.content || hunk.Content || '';
            const contentLines = content.split('\n');
            let oldLine = oldStartLine;
            let newLine = newStartLine;
            
            const lines = [];
            for (const line of contentLines) {
                if (!line || line.startsWith('@@')) continue;
                
                let lineData;
                if (line.startsWith('-')) {
                    const lineComments = takeCommentsForLine(oldLine);
                    lineData = {
                        OldNum: String(oldLine),
                        NewNum: '',
                        Content: line,
                        Class: 'diff-del',
                        IsComment: lineComments.length > 0,
                        Comments: lineComments
                    };
                    oldLine++;
                } else if (line.startsWith('+')) {
                    const lineComments = takeCommentsForLine(newLine);
                    lineData = {
                        OldNum: '',
                        NewNum: String(newLine),
                        Content: line,
                        Class: 'diff-add',
                        IsComment: lineComments.length > 0,
                        Comments: lineComments
                    };
                    newLine++;
                } else {
                    const lineComments = takeCommentsForLine(newLine);
                    lineData = {
                        OldNum: String(oldLine),
                        NewNum: String(newLine),
                        Content: ' ' + line,
                        Class: 'diff-context',
                        IsComment: lineComments.length > 0,
                        Comments: lineComments
                    };
                    oldLine++;
                    newLine++;
                }
                lines.push(lineData);
            }
            
            return { Header: header, Lines: lines };
        });
        
        return {
            ID: fileId,
            FilePath: filePath,
            HasComments: comments.length > 0,
            CommentCount: comments.length,
            Hunks: processedHunks
        };
    });
}

function getRawFiles(payload, previousRawFiles) {
    if (Array.isArray(payload?.files)) {
        return payload.files;
    }
    if (Array.isArray(previousRawFiles)) {
        return previousRawFiles;
    }
    return [];
}

function getReviewID(payload) {
    return payload?.reviewID || payload?.ReviewID || '';
}

function getReviewStatus(payload) {
    return payload?.status || payload?.Status || '';
}

function withDerivedReviewFields(next, prev, setExpandedFiles) {
    if (!next) return next;

    const rawFiles = getRawFiles(next, prev?.files);
    const uiFiles = convertFilesToUIFormat(rawFiles);
    const actualCommentCount = countCommentsFromFiles(rawFiles);

    if (!prev) {
        const expanded = new Set();
        uiFiles.forEach(file => {
            if (file.HasComments) {
                expanded.add(file.ID);
            }
        });
        if (expanded.size > 0) {
            setExpandedFiles(expanded);
        }
    } else {
        const prevCommentCounts = new Map((prev.Files || []).map(file => [file.FilePath, file.CommentCount || 0]));
        const filesNeedingExpansion = uiFiles.filter(file => (prevCommentCounts.get(file.FilePath) || 0) === 0 && file.CommentCount > 0);
        if (filesNeedingExpansion.length > 0) {
            setExpandedFiles(prevExpanded => {
                const nextExpanded = new Set(prevExpanded);
                filesNeedingExpansion.forEach(file => nextExpanded.add(file.ID));
                return nextExpanded;
            });
        }
    }

    return {
        ...next,
        files: rawFiles,
        Files: uiFiles,
        TotalFiles: uiFiles.length,
        TotalComments: actualCommentCount
    };
}

async function initApp() {
    const { h, render, useState, useEffect, useCallback, useRef, html } = await waitForPreact();
    
    // Load all components
    const Header = await getHeader();
    const Sidebar = await getSidebar();
    const Summary = await getSummary();
    const Stats = await getStats();
    const PrecommitBar = await getPrecommitBar();
    const FileBlock = await getFileBlock();
    const EventLog = await getEventLog();
    const IssueFilterBar = await getIssueFilterBar();
    const Toolbar = await getToolbar();
    const CommentNav = await getCommentNav();
    const SummarySlideshow = await getSummarySlideshow();
    
    function App() {
        // Core data state - fetched from API
        const [reviewData, setReviewData] = useState(null);
        const [loading, setLoading] = useState(true);
        const [error, setError] = useState(null);
        
        // UI state
        const [activeTab, setActiveTab] = useState('files');
        const [expandedFiles, setExpandedFiles] = useState(new Set());
        const [allExpanded, setAllExpanded] = useState(false);
        const [activeFileId, setActiveFileId] = useState(null);
        const [issueFilters, setIssueFilters] = useState(createDefaultIssueFilters());
        const [events, setEvents] = useState([]);
        const [newEventCount, setNewEventCount] = useState(0);
        const [isTailing, setIsTailing] = useState(false);
        const [hiddenCommentKeys, setHiddenCommentKeys] = useState(new Set());
        const [copyFeedback, setCopyFeedback] = useState({ status: 'idle', message: '' });
        const [handoffModal, setHandoffModal] = useState({ isOpen: false, type: '', message: '', agent: null, summary: null });
        const [slideShowOpen, setSlideShowOpen] = useState(false);
        const [embeddedSlideshowActive, setEmbeddedSlideshowActive] = useState(false);
        const [summarySlideIndex, setSummarySlideIndex] = useState(0);
        const [performanceNowMs, setPerformanceNowMs] = useState(domReadyStartMs || getPerformanceNow());
        const [commentRenderTimes, setCommentRenderTimes] = useState({});
        const [commentVotes, setCommentVotes] = useState({});
        
        const eventsPollingRef = useRef(null);
        const eventsListRef = useRef(null);
        const copyFeedbackTimerRef = useRef(null);
        const activeTabRef = useRef('files');
        const seenEventIdsRef = useRef(new Set());
        const lastSeenEventTimeRef = useRef(null);
        const finalFetchStartedRef = useRef(false);
        const eventsInFlightRef = useRef(false);
        const reviewStartMsRef = useRef(domReadyStartMs || getPerformanceNow());
        const reviewCompletedMsRef = useRef(null);
        const [logsCopied, setLogsCopied] = useState(false);
        const [sessionEnded, setSessionEnded] = useState(false);
        const sessionReviewID = new URLSearchParams(window.location.search).get('r') || '';

        useEffect(() => {
            activeTabRef.current = activeTab;
        }, [activeTab]);

        const commitReviewData = useCallback((updater) => {
            setReviewData(prev => {
                const next = typeof updater === 'function' ? updater(prev) : updater;
                return withDerivedReviewFields(next, prev, setExpandedFiles);
            });
        }, []);
        
        // Fetch review data from API
        const fetchInitialReviewData = useCallback(async () => {
            try {
                const url = sessionReviewID ? `/api/review?r=${sessionReviewID}` : '/api/review';
                const response = await fetch(url);
                if (response.status === 403) {
                    setSessionEnded(true);
                    setLoading(false);
                    return null;
                }
                if (!response.ok) {
                    throw new Error(`Failed to fetch review data: ${response.status}`);
                }
                const data = await response.json();
                commitReviewData(data);
                setReviewMeta({ apiURL: data.apiURL || data.APIURL || '', reviewID: data.reviewID || data.ReviewID || '' });
                setLoading(false);
                return data;
            } catch (err) {
                console.error('Error fetching review data:', err);
                setError(err.message);
                setLoading(false);
                return null;
            }
        }, [commitReviewData, sessionReviewID]);

        const fetchFinalReviewData = useCallback(async (reviewID) => {
            if (!reviewID) return null;
            const rParam = sessionReviewID ? `?r=${sessionReviewID}` : '';
            try {
                const response = await fetch(`/api/v1/diff-review/${reviewID}${rParam}`);
                if (!response.ok) return null;
                const data = await response.json();
                commitReviewData(prev =>
                    prev ? { ...prev, ...data, files: data.files || prev.files || [] } : data
                );
                return data;
            } catch (err) {
                console.error('Error fetching final review data:', err);
                return null;
            }
        }, [commitReviewData, sessionReviewID]);
        
        // Fetch events for live logs and comments
        const fetchEvents = useCallback(async (reviewID) => {
            if (!reviewID) return;
            if (eventsInFlightRef.current) return;
            eventsInFlightRef.current = true;
            
            try {
                const baseEventsURL = buildEventsURL(reviewID, lastSeenEventTimeRef.current);
                const eventsURL = sessionReviewID ? `${baseEventsURL}&r=${sessionReviewID}` : baseEventsURL;
                const response = await fetch(eventsURL);
                if (response.status === 403) {
                    setSessionEnded(true);
                    return;
                }
                if (!response.ok) return;
                
                const data = await response.json();
                const backendEvents = data.events || [];
                const { newEvents, nextSeenEventIds, lastSeenAt } = extractNewEvents(backendEvents, seenEventIdsRef.current);

                if (lastSeenAt) {
                    lastSeenEventTimeRef.current = lastSeenAt;
                }
                if (newEvents.length === 0 && !data.meta?.status) {
                    return;
                }

                seenEventIdsRef.current = nextSeenEventIds;

                const transformedEvents = newEvents.map(transformEvent);
                if (transformedEvents.length > 0) {
                    setEvents(prev => {
                        if (activeTabRef.current !== 'events') {
                            setNewEventCount(count => count + transformedEvents.length);
                        }
                        return prev.concat(transformedEvents);
                    });
                }

                const streamedComments = extractExternalCommentsFromEvents(newEvents);
                const liveStatus = data.meta?.status || inferReviewStatusFromEvents(newEvents);

                if (streamedComments.length > 0 || liveStatus) {
                    commitReviewData(prev => {
                        if (!prev) return prev;

                        const next = { ...prev };
                        if (liveStatus) {
                            next.status = liveStatus;
                            next.Status = liveStatus;
                        }
                        if (streamedComments.length > 0) {
                            next.files = appendStreamedCommentsToFiles(prev.files || [], streamedComments);
                        }
                        return next;
                    });
                }

                if ((liveStatus === 'completed' || liveStatus === 'failed') && !finalFetchStartedRef.current) {
                    finalFetchStartedRef.current = true;
                    if (eventsPollingRef.current) {
                        clearInterval(eventsPollingRef.current);
                        eventsPollingRef.current = null;
                    }
                    await fetchFinalReviewData(reviewID);
                }
            } catch (err) {
                console.error('Error fetching events:', err);
            } finally {
                eventsInFlightRef.current = false;
            }
        }, [commitReviewData, fetchFinalReviewData, sessionReviewID]);
        
        // Initial load and polling setup
        useEffect(() => {
            let cancelled = false;

            const start = async () => {
                const data = await fetchInitialReviewData();
                if (cancelled || !data) {
                    return;
                }

                const reviewID = getReviewID(data);
                const status = getReviewStatus(data);
                if (reviewID) {
                    await fetchEvents(reviewID);
                }
                if (cancelled || !reviewID || finalFetchStartedRef.current || status === 'completed' || status === 'failed') {
                    return;
                }

                eventsPollingRef.current = setInterval(() => {
                    fetchEvents(reviewID);
                }, 1000);
            };

            start();
            
            // Cleanup
            return () => {
                cancelled = true;
                if (eventsPollingRef.current) {
                    clearInterval(eventsPollingRef.current);
                    eventsPollingRef.current = null;
                }
            };
        }, [fetchInitialReviewData, fetchEvents]);
        
        // Update page title with friendly name
        useEffect(() => {
            if (reviewData?.friendlyName) {
                document.title = `LiveReview - ${reviewData.friendlyName}`;
            } else {
                document.title = 'LiveReview';
            }
        }, [reviewData?.friendlyName]);
        
        // Toggle single file
        const toggleFile = useCallback((fileId) => {
            setExpandedFiles(prev => {
                const next = new Set(prev);
                if (next.has(fileId)) {
                    next.delete(fileId);
                } else {
                    next.add(fileId);
                }
                return next;
            });
        }, []);
        
        // Toggle all files
        const toggleAll = useCallback(() => {
            if (allExpanded) {
                setExpandedFiles(new Set());
                setAllExpanded(false);
            } else {
                const all = new Set();
                (reviewData?.Files || []).forEach(file => {
                    all.add(file.ID);
                });
                setExpandedFiles(all);
                setAllExpanded(true);
            }
        }, [allExpanded, reviewData?.Files]);
        
        // Handle sidebar file click
        const handleFileClick = useCallback((fileId, lineNumber = null) => {
            // Always switch to files tab when clicking a file in sidebar
            setActiveTab('files');
            setActiveFileId(fileId);
            setExpandedFiles(prev => {
                const next = new Set(prev);
                next.add(fileId);
                return next;
            });
            
            // Scroll to file after brief delay to allow tab switch
            setTimeout(() => {
                const fileEl = document.getElementById(fileId);
                if (fileEl) {
                    const mainContent = document.querySelector('.main-content');
                    const header = document.querySelector('.header');
                    const headerHeight = header ? header.offsetHeight : 60;

                    const parsedLine = Number(lineNumber);
                    const hasTargetLine = Number.isFinite(parsedLine) && parsedLine > 0;
                    const lineSelector = hasTargetLine
                        ? `.diff-line[data-new-line="${parsedLine}"] , .diff-line[data-old-line="${parsedLine}"]`
                        : '';
                    const targetLineEl = hasTargetLine ? fileEl.querySelector(lineSelector) : null;

                    if (targetLineEl && mainContent) {
                        const lineRect = targetLineEl.getBoundingClientRect();
                        const mainContentRect = mainContent.getBoundingClientRect();
                        const scrollTarget = mainContent.scrollTop + lineRect.top - mainContentRect.top - headerHeight - 14;
                        mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                        targetLineEl.classList.add('line-highlight');
                        setTimeout(() => targetLineEl.classList.remove('line-highlight'), 1800);
                        return;
                    }

                    const fileRect = fileEl.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + fileRect.top - mainContentRect.top - headerHeight - 10;
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                }
            }, 100);
        }, []);

        const resolveSlideFileId = useCallback((filePath) => {
            const normalized = (filePath || '').trim();
            if (!normalized) {
                return null;
            }

            const reviewFiles = reviewData?.Files || [];
            if (!reviewFiles.length) {
                return null;
            }

            const exact = reviewFiles.find(file => (file?.FilePath || '') === normalized);
            if (exact) {
                return exact.ID || filePathToId(exact.FilePath || normalized);
            }

            const normalizedLower = normalized.toLowerCase();
            const suffixMatches = reviewFiles.filter(file => {
                const candidate = (file?.FilePath || '').toLowerCase();
                if (!candidate) {
                    return false;
                }
                return candidate === normalizedLower || candidate.endsWith(`/${normalizedLower}`);
            });

            if (suffixMatches.length !== 1) {
                return null;
            }

            const matched = suffixMatches[0];
            return matched.ID || filePathToId(matched.FilePath || normalized);
        }, [reviewData]);

        const canOpenFileFromSlide = useCallback((filePath) => {
            return Boolean(resolveSlideFileId(filePath));
        }, [resolveSlideFileId]);

        const handleOpenFileFromSlide = useCallback((filePath, lineNumber = null) => {
            if (!filePath) {
                return false;
            }

            const fileId = resolveSlideFileId(filePath);
            if (!fileId) {
                return false;
            }

            setSlideShowOpen(false);
            handleFileClick(fileId, lineNumber);
            return true;
        }, [handleFileClick, resolveSlideFileId]);

        const getVisibleTopContentOffset = useCallback((mainContent) => {
            if (!mainContent) {
                return 0;
            }

            const mainContentRect = mainContent.getBoundingClientRect();
            const topAnchors = ['.header', '.issue-filter-bar'];

            return topAnchors.reduce((maxOffset, selector) => {
                const element = document.querySelector(selector);
                if (!element) {
                    return maxOffset;
                }

                const rect = element.getBoundingClientRect();
                const overlapsViewport = rect.bottom > mainContentRect.top && rect.top < mainContentRect.bottom;
                if (!overlapsViewport) {
                    return maxOffset;
                }

                return Math.max(maxOffset, rect.bottom - mainContentRect.top);
            }, 0);
        }, []);
        
        // Navigate to comment
        const navigateToComment = useCallback((commentId, fileId) => {
            // Switch to files tab first
            setActiveTab('files');
            
            // Expand the file containing the comment
            setExpandedFiles(prev => {
                const next = new Set(prev);
                next.add(fileId);
                return next;
            });
            
            setTimeout(() => {
                const comment = document.getElementById(commentId);
                if (comment) {
                    const mainContent = document.querySelector('.main-content');
                    if (!mainContent) {
                        return;
                    }

                    const commentBox = comment.querySelector('.comment-container, .comment-hidden-placeholder') || comment;
                    const commentRect = commentBox.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const topOffset = getVisibleTopContentOffset(mainContent);
                    const readableGapPx = 18;
                    const scrollTarget = mainContent.scrollTop + commentRect.top - mainContentRect.top - topOffset - readableGapPx;
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                    
                    comment.classList.add('highlight');
                    setTimeout(() => comment.classList.remove('highlight'), 1500);
                }
            }, 100);
        }, [getVisibleTopContentOffset]);
        
        // Tab change
        const handleTabChange = useCallback((tab) => {
            setActiveTab(tab);
            if (tab === 'events') {
                setNewEventCount(0);
            }
        }, []);

        const handleVote = useCallback((visibilityKey, newVote) => {
            if (!visibilityKey) return;
            setCommentVotes(prev => {
                if (newVote === null) {
                    const next = { ...prev };
                    delete next[visibilityKey];
                    return next;
                }
                return { ...prev, [visibilityKey]: newVote };
            });
        }, []);

        const toggleCommentVisibility = useCallback((visibilityKey) => {
            if (!visibilityKey) {
                console.warn('Cannot toggle comment visibility without a key');
                return;
            }
            setHiddenCommentKeys(prev => {
                const next = new Set(prev);
                if (next.has(visibilityKey)) {
                    next.delete(visibilityKey);
                } else {
                    next.add(visibilityKey);
                }
                return next;
            });
        }, []);

        const handleSendToAgent = useCallback(async (agentKey) => {
            const agent = getSendToAgentInfo(agentKey);

            if (!agent.available) {
                setHandoffModal({
                    isOpen: true,
                    type: 'info',
                    agent,
                    message: `${agent.label} handoff is coming soon. Try Claude for now.`
                });
                return;
            }

            const filteredFiles = (reviewData.files || reviewData.Files || []).map(file => {
                const filePath = file.file_path || file.filePath || file.FilePath;
                const newComments = (file.comments || file.Comments || []).filter(c => {
                    if (!matchesIssueFilters(c, issueFilters)) return false;
                    const key = getCommentVisibilityKey(filePath, c);
                    return !hiddenCommentKeys.has(key);
                });
                return { ...file, comments: newComments, Comments: newComments };
            }).filter(file => file.comments.length > 0);

            if (filteredFiles.length === 0) {
                setHandoffModal({
                    isOpen: true,
                    type: 'error',
                    agent,
                    message: "No visible comments to send to the AI agent. Please show some comments first."
                });
                return;
            }

            const impactSummary = buildHandoffImpactSummary(filteredFiles);

            setHandoffModal({
                isOpen: true,
                type: 'starting',
                agent,
                summary: impactSummary,
                message: `${agent.label} started auto-fixing the issues…`
            });

            const payload = {
                ...reviewData,
                files: filteredFiles,
                Files: filteredFiles,
                summary: "AI Agent Handoff generated for visible issues.",
                status: "completed",
                agent: agent.key
            };

            try {
                const handoffURL = sessionReviewID ? `/handoff?r=${sessionReviewID}` : '/handoff';
                const response = await fetch(handoffURL, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (!response.ok) throw new Error("Handoff failed");
                setHandoffModal({
                    isOpen: true,
                    type: 'success',
                    agent,
                    summary: impactSummary,
                    message: `Auto-fixing started for your issues — check the ${agent.label} Code terminal to follow along. You can safely close this browser window.`
                });
            } catch (e) {
                setHandoffModal({
                    isOpen: true,
                    type: 'error',
                    agent,
                    message: "Failed to send to agent: " + e.message
                });
            }
        }, [reviewData, issueFilters, hiddenCommentKeys]);

        const showCopyFeedback = useCallback((status, message) => {
            setCopyFeedback({ status, message });
            if (copyFeedbackTimerRef.current) {
                clearTimeout(copyFeedbackTimerRef.current);
                copyFeedbackTimerRef.current = null;
            }
            if (status !== 'idle') {
                copyFeedbackTimerRef.current = setTimeout(() => {
                    setCopyFeedback({ status: 'idle', message: '' });
                    copyFeedbackTimerRef.current = null;
                }, 2500);
            }
        }, []);

        const handleCommentRendered = useCallback((commentKey) => {
            const renderMs = getPerformanceNow();
            setCommentRenderTimes((prev) => recordFirstRenderTime(prev, commentKey, renderMs));
        }, []);

        useEffect(() => {
            return () => {
                if (copyFeedbackTimerRef.current) {
                    clearTimeout(copyFeedbackTimerRef.current);
                    copyFeedbackTimerRef.current = null;
                }
            };
        }, []);

        const status = reviewData?.status || 'in_progress';
        const showLoader = Boolean(reviewData) && status === 'in_progress';
        const summary = reviewData?.summary || '';
        const files = reviewData?.Files || [];
        const totalComments = files.reduce((sum, file) => sum + (file.CommentCount || 0), 0);
        const errorSummary = reviewData?.errorSummary || '';
        const hasSummary = Boolean(summary && summary.trim());
        const summarySlidesEligibility = hasSummary ? evaluateSummarySlidesEligibility(summary) : { eligible: false, reason: 'empty-summary' };
        const showAllClear = shouldShowAllClear({ status, totalComments, errorSummary, summarySlidesEligibility });
        const slidesEnabled = Boolean(summarySlidesEligibility.eligible && hasSummary);
        const firstCommentRenderMs = getFirstRenderTime(commentRenderTimes);
        const performanceSnapshot = buildPerformanceSnapshot({
            baselineMs: reviewStartMsRef.current,
            nowMs: performanceNowMs,
            firstCommentMs: firstCommentRenderMs,
            totalComments,
            completedMs: reviewCompletedMsRef.current,
        });
        const loadingActivityMessage = getLoadingActivityMessage(events, performanceSnapshot.elapsedMs);
        const loaderHeadline = firstCommentRenderMs === null ? 'Review in progress' : 'Comments are still streaming';
        const loaderMeta = firstCommentRenderMs === null
            ? `Elapsed ${performanceSnapshot.elapsedLabel} • first comment pending`
            : `First comment in ${performanceSnapshot.firstCommentLabel} • ${totalComments} comment${totalComments !== 1 ? 's' : ''} so far`;

        useEffect(() => {
            if (!slidesEnabled && slideShowOpen) {
                setSlideShowOpen(false);
            }
        }, [slidesEnabled, slideShowOpen]);

        useEffect(() => {
            if (status === 'completed' || status === 'failed') {
                if (reviewCompletedMsRef.current === null) {
                    const completedMs = getPerformanceNow();
                    reviewCompletedMsRef.current = completedMs;
                    setPerformanceNowMs(completedMs);
                }
                return undefined;
            }

            const interval = setInterval(() => {
                setPerformanceNowMs(getPerformanceNow());
            }, 1000);

            return () => {
                clearInterval(interval);
            };
        }, [status]);
        
        // Tail log handler - toggle tailing on/off
        const handleTailLog = useCallback(() => {
            setIsTailing(prev => {
                const newValue = !prev;
                if (newValue && eventsListRef.current) {
                    eventsListRef.current.scrollTop = eventsListRef.current.scrollHeight;
                }
                return newValue;
            });
        }, []);
        
        // Copy logs handler
        const handleCopyLogs = useCallback(async () => {
            const logsText = events.map((event, index) => {
                const time = event.time ? new Date(event.time).toLocaleTimeString() : '';
                const type = event.type ? event.type.toUpperCase() : 'LOG';
                return `[${index + 1}] ${time} - ${type}\n  ${event.message}`;
            }).join('\n\n');
            
            try {
                await navigator.clipboard.writeText(logsText);
                setLogsCopied(true);
                setTimeout(() => setLogsCopied(false), 2000);
            } catch (err) {
                console.error('Failed to copy logs:', err);
            }
        }, [events]);
        
        // Session mismatch — stale tab refreshed on a reused port
        if (sessionEnded) {
            return html`
                <div class="loading-screen">
                    <div class="loading-content">
                        <div class="loading-logo error">
                            ${renderIcon(html, 'handoffNotice', { size: 48 })}
                        </div>
                        <h1 class="loading-title">Session Ended</h1>
                        <p class="loading-text">This review session is no longer active.</p>
                        <p class="loading-text" style="font-size:13px;opacity:0.7">Close this tab and open the new review from your terminal.</p>
                    </div>
                </div>
            `;
        }

        // Loading state
        if (loading && !reviewData) {
            return html`
                <div class="loading-screen">
                    <div class="loading-content">
                        <div class="loading-logo">
                            ${renderIcon(html, 'refresh', { size: 48, className: 'ui-icon-spin' })}
                        </div>
                        <h1 class="loading-title">LiveReview</h1>
                        <div class="loading-spinner"></div>
                        <p class="loading-text">Loading review data...</p>
                    </div>
                </div>
            `;
        }
        
        // Error state
        if (error && !reviewData) {
            return html`
                <div class="loading-screen">
                    <div class="loading-content">
                        <div class="loading-logo error">
                            ${renderIcon(html, 'errorStatus', { size: 48 })}
                        </div>
                        <h1 class="loading-title">LiveReview</h1>
                        <h2 class="loading-error-title">Error Loading Review</h2>
                        <p class="loading-error-text">${error}</p>
                    </div>
                </div>
            `;
        }
        
        const filterCounts = countIssuesByFilters(files, issueFilters, hiddenCommentKeys);
        const filterOptions = buildIssueFacetOptions(files, issueFilters, hiddenCommentKeys);
        const categoryGroups = buildIssueCategoryGroups(files, issueFilters, hiddenCommentKeys);
        const allCategoryGroups = buildIssueCategoryGroups(files, createDefaultIssueFilters(), hiddenCommentKeys);
        const filterUniverse = buildIssueFilterUniverse(files, hiddenCommentKeys);
        const filterSummary = getIssueFilterSummary(issueFilters, filterUniverse);

        const toggleIssueFilter = useCallback((field, value) => {
            const matchingGroup = field === 'category'
                ? allCategoryGroups.find((group) => group.value === String(value || '').trim().toLowerCase())
                : null;
            setIssueFilters((prev) => toggleIssueFilterValue(prev, field, value, {
                childValues: matchingGroup?.subcategories?.map((subcategory) => subcategory.value) || [],
            }));
        }, [allCategoryGroups]);

        const handleResetIssueFilters = useCallback(() => {
            setIssueFilters(resetIssueFilters());
        }, []);
        
        // Copy all visible issues to clipboard
        const handleCopyVisibleIssues = useCallback(async () => {
            const lines = [];
            files.forEach(file => {
                if (!file.HasComments) return;
                file.Hunks.forEach(hunk => {
                    hunk.Lines.forEach(line => {
                        if (line.IsComment && line.Comments) {
                            line.Comments.forEach((comment) => {
                                if (!matchesIssueFilters(comment, issueFilters)) return;
                                const visibilityKey = getCommentVisibilityKey(file.FilePath, comment);
                                if (visibilityKey && hiddenCommentKeys.has(visibilityKey)) return;
                                lines.push(formatIssueForCopy(file.FilePath, comment));
                            });
                        }
                    });
                });
            });
            if (lines.length === 0) {
                showCopyFeedback('empty', 'No visible issues to copy');
                return;
            }
            try {
                const numbered = lines.map((text, idx) => `${idx + 1}. ${text}`).join('\n\n');
                await navigator.clipboard.writeText(numbered);
                showCopyFeedback('success', `Copied ${lines.length} issue${lines.length !== 1 ? 's' : ''}`);
            } catch (err) {
                console.error('Failed to copy issues:', err);
                showCopyFeedback('error', 'Failed to copy issues');
            }
        }, [files, issueFilters, hiddenCommentKeys, showCopyFeedback]);
        
        // Build flat ordered list of VISIBLE comments for navigation
        const allComments = [];
        const commentIds = [];
        files.forEach(file => {
            const fileId = file.ID || filePathToId(file.FilePath);
            file.Hunks.forEach(hunk => {
                hunk.Lines.forEach(line => {
                    if (line.IsComment && line.Comments) {
                        line.Comments.forEach((comment, commentIdx) => {
                            if (!matchesIssueFilters(comment, issueFilters)) return;
                            const visibilityKey = getCommentVisibilityKey(file.FilePath, comment);
                            if (visibilityKey && hiddenCommentKeys.has(visibilityKey)) return;
                            const cid = `comment-${fileId}-${comment.Line}-${commentIdx}`;
                            allComments.push({
                                filePath: file.FilePath,
                                fileId: fileId,
                                line: comment.Line,
                                commentId: cid
                            });
                            commentIds.push(cid);
                        });
                    }
                });
            });
        });
        // Stable key that only changes when the actual comment set changes
        const commentKey = commentIds.join(',');
        
        // Calculate visible comments for the agent button
        let totalVisibleComments = 0;
        files.forEach(file => {
            if (!file.HasComments) return;
            file.Hunks.forEach(hunk => {
                hunk.Lines.forEach(line => {
                    if (line.IsComment && line.Comments) {
                        line.Comments.forEach((comment) => {
                            if (!matchesIssueFilters(comment, issueFilters)) return;
                            const visibilityKey = getCommentVisibilityKey(file.FilePath, comment);
                            if (visibilityKey && hiddenCommentKeys.has(visibilityKey)) return;
                            totalVisibleComments++;
                        });
                    }
                });
            });
        });
        
        // Status display
        const getStatusDisplay = () => {
            if (reviewData?.blocked) {
                return null;
            }
            if (status === 'failed') {
                return html`
                    <div class="status-container error">
                        <span class="status-icon">${renderIcon(html, 'errorStatus', { size: 16 })}</span>
                        <span>Review completed with errors</span>
                    </div>
                `;
            }
            if (status === 'completed') {
                return html`
                    <div class="status-container success">
                        <span class="status-icon">${renderIcon(html, 'successStatus', { size: 16 })}</span>
                        <span>Review completed successfully</span>
                    </div>
                `;
            }
            return null;
        };
        
        return html`
            <${Sidebar} 
                files=${files}
                activeFileId=${activeFileId}
                onFileClick=${handleFileClick}
                issueFilters=${issueFilters}
                hiddenCommentKeys=${hiddenCommentKeys}
            />
            <div class="main-content">
                <div class="container">
                    <${Header} 
                        generatedTime=${reviewData?.generatedTime || reviewData?.GeneratedTime}
                        friendlyName=${reviewData?.friendlyName || reviewData?.FriendlyName}
                        repositoryPath=${reviewData?.repositoryPath || reviewData?.RepositoryPath}
                    />
                    
                    ${showLoader && html`
                        <div class="loader-container">
                            <div class="loader-content">
                                <div class="spinner"></div>
                                <div class="loader-copy">
                                    <span class="loader-message">${loaderHeadline}</span>
                                    <span class="loader-detail">${loadingActivityMessage}</span>
                                    <span class="loader-meta">${loaderMeta}</span>
                                </div>
                            </div>
                        </div>
                    `}
                    
                    ${getStatusDisplay()}
                    
                    <${UsageBanner} endpoint="/api/runtime/usage-chip" />
                    
                    ${(showAllClear || (summary && summary.trim())) && status !== 'in_progress' && html`
                        <${Summary} 
                            markdown=${summary}
                            status=${status}
                            errorSummary=${errorSummary}
                            showAllClear=${showAllClear}
                            slidesEnabled=${slidesEnabled}
                            isSlideshowModalOpen=${slideShowOpen}
                            onOpenSlideshowModal=${() => setSlideShowOpen(true)}
                            onEmbeddedShortcutActiveChange=${setEmbeddedSlideshowActive}
                            slideIndex=${summarySlideIndex}
                            onSlideIndexChange=${setSummarySlideIndex}
                            onOpenFileFromSlide=${handleOpenFileFromSlide}
                            canOpenFileFromSlide=${canOpenFileFromSlide}
                        />
                    `}
                    
                    <${Stats} 
                        totalFiles=${files.length}
                        totalComments=${totalComments}
                    />
                    
                    <${PrecommitBar}
                        interactive=${reviewData?.interactive || reviewData?.Interactive}
                        isPostCommitReview=${reviewData?.isPostCommitReview || reviewData?.IsPostCommitReview}
                        initialMsg=${reviewData?.initialMsg || reviewData?.InitialMsg || ''}
                        summary=${summary}
                        status=${status}
                    />
                    
                    <${Toolbar}
                        activeTab=${activeTab}
                        onTabChange=${handleTabChange}
                        performanceItems=${performanceSnapshot.summaryItems}
                        allExpanded=${allExpanded}
                        onToggleAll=${toggleAll}
                        eventCount=${newEventCount}
                        showEventBadge=${activeTab !== 'events'}
                        onTailLog=${handleTailLog}
                        isTailing=${isTailing}
                        onCopyLogs=${handleCopyLogs}
                        logsCopied=${logsCopied}
                    />
                    
                    ${activeTab === 'files' && html`
                        <${IssueFilterBar}
                            files=${files}
                            issueFilters=${issueFilters}
                            filterOptions=${filterOptions}
                            categoryGroups=${categoryGroups}
                            filterCounts=${filterCounts}
                            filterSummary=${filterSummary}
                            onToggleFilter=${toggleIssueFilter}
                            onResetFilters=${handleResetIssueFilters}
                            onCopyVisibleIssues=${handleCopyVisibleIssues}
                            hiddenCommentKeys=${hiddenCommentKeys}
                            copyFeedbackStatus=${copyFeedback.status}
                            copyFeedbackMessage=${copyFeedback.message}
                            onSendToAgent=${handleSendToAgent}
                            visibleCount=${totalVisibleComments}
                            prVote=${commentVotes['__pr_level__'] || null}
                            onVote=${handleVote}
                        />
                    `}
                    
                    <!-- Files Tab -->
                    <div id="files-tab" class="tab-content ${activeTab === 'files' ? 'active' : ''}" style="display: ${activeTab === 'files' ? 'block' : 'none'}">
                        ${files.length > 0 
                            ? files.map(file => html`
                                <${FileBlock}
                                    key=${file.ID}
                                    file=${file}
                                    expanded=${expandedFiles.has(file.ID)}
                                    onToggle=${toggleFile}
                                    issueFilters=${issueFilters}
                                    hiddenCommentKeys=${hiddenCommentKeys}
                                    onToggleCommentVisibility=${toggleCommentVisibility}
                                    reviewStartMs=${reviewStartMsRef.current}
                                    commentRenderTimes=${commentRenderTimes}
                                    onCommentRendered=${handleCommentRendered}
                                    commentVotes=${commentVotes}
                                    onVote=${handleVote}
                                />
                            `)
                            : html`
                                <div style="padding: 40px 20px; text-align: center; color: #57606a;">
                                    ${status === 'in_progress' 
                                        ? 'Waiting for review results...' 
                                        : 'No files reviewed or no comments generated.'}
                                </div>
                            `
                        }
                    </div>
                    
                    ${handoffModal.isOpen && handoffModal.type === 'success' && renderHandoffConfetti(html)}
                    ${handoffModal.isOpen && html`
                        <div class="modal-overlay handoff-modal-overlay">
                            <div class="modal-content handoff-modal-content">
                                ${(handoffModal.type === 'starting' || handoffModal.type === 'success')
                                    ? html`
                                        <div class="handoff-agent-badge ${handoffModal.type === 'success' ? 'is-success' : ''}">
                                            ${handoffModal.type === 'starting' && html`<span class="handoff-agent-badge-ring"></span>`}
                                            ${(handoffModal.agent?.logoOnLight || handoffModal.agent?.logo)
                                                ? html`<img class="handoff-agent-badge-logo" src=${handoffModal.agent.logoOnLight || handoffModal.agent.logo} alt="${handoffModal.agent.label}" />`
                                                : renderIcon(html, handoffModal.agent?.icon || 'sendToAgent', { size: 26 })}
                                            ${handoffModal.type === 'success' && html`
                                                <span class="handoff-agent-badge-check">${renderIcon(html, 'check', { size: 12 })}</span>
                                            `}
                                        </div>
                                    `
                                    : html`<div style="margin-bottom: 16px;">${renderIcon(html, 'handoffNotice', { size: 48 })}</div>`
                                }
                                <h3 style="margin: 0 0 12px 0; font-size: 20px; color: var(--text-primary);">
                                    ${handoffModal.type === 'starting'
                                        ? `${handoffModal.agent?.label || 'Agent'} is on it`
                                        : handoffModal.type === 'success'
                                            ? `Auto-fixing Using ${handoffModal.agent?.label || 'Agent'} Code`
                                            : handoffModal.type === 'info'
                                                ? 'Coming Soon'
                                                : 'Notice'}
                                </h3>
                                <p style="margin: 0 0 16px 0; color: var(--text-secondary); line-height: 1.5;">
                                    ${handoffModal.message}
                                </p>
                                ${handoffModal.summary && handoffModal.summary.total > 0 && html`
                                    <div class="handoff-impact-summary">
                                        <div class="handoff-impact-total">
                                            ${handoffModal.summary.total} issue${handoffModal.summary.total === 1 ? '' : 's'} being fixed
                                        </div>
                                        <div class="handoff-impact-section">
                                            <div class="handoff-impact-label">Severity</div>
                                            <div class="handoff-impact-chips">
                                                ${['critical', 'warning', 'info']
                                                    .filter((severity) => handoffModal.summary.bySeverity[severity] > 0)
                                                    .map((severity) => html`
                                                        <span class="handoff-impact-chip severity-${severity}">
                                                            <span class="handoff-impact-chip-dot"></span>
                                                            ${handoffModal.summary.bySeverity[severity]} ${severity.charAt(0).toUpperCase() + severity.slice(1)}
                                                        </span>
                                                    `)}
                                            </div>
                                        </div>
                                        ${handoffModal.summary.byType && handoffModal.summary.byType.length > 0 && html`
                                            <div class="handoff-impact-section handoff-impact-section-divided">
                                                <div class="handoff-impact-label">Type</div>
                                                <div class="handoff-impact-types">
                                                    ${handoffModal.summary.byType.map((entry) => html`
                                                        <span class="handoff-impact-type-chip">
                                                            ${entry.label}
                                                            <span class="handoff-impact-type-count">${entry.count}</span>
                                                        </span>
                                                    `)}
                                                </div>
                                            </div>
                                        `}
                                    </div>
                                `}
                                <button
                                    class="btn btn-primary"
                                    onClick=${() => setHandoffModal({ ...handoffModal, isOpen: false })}
                                    style="width: 100%; padding: 12px; font-size: 16px;"
                                >
                                    ${handoffModal.type === 'success' ? 'Got it' : 'Close'}
                                </button>
                            </div>
                        </div>
                    `}
                    
                    <!-- Events Tab -->
                    <div id="events-tab" class="tab-content ${activeTab === 'events' ? 'active' : ''}" style="display: ${activeTab === 'events' ? 'block' : 'none'}">
                        <${EventLog}
                            events=${events}
                            status=${status}
                            isTailing=${isTailing}
                            listRef=${eventsListRef}
                        />
                    </div>
                    
                    <div class="footer">
                        ${status === 'in_progress' 
                            ? `Review in progress: ${totalComments} comment(s) so far`
                            : `Review complete: ${totalComments} total comment(s)`
                        }
                    </div>
                </div>
            </div>
            <${CommentNav}
                allComments=${allComments}
                commentKey=${commentKey}
                onNavigate=${navigateToComment}
                activeTab=${activeTab}
                slideshowOpen=${slideShowOpen}
                embeddedSlideshowActive=${embeddedSlideshowActive}
            />
            
            ${slidesEnabled && html`
                <${SummarySlideshow}
                    markdown=${summary}
                    isOpen=${slideShowOpen}
                    mode="modal"
                    initialSlideIndex=${summarySlideIndex}
                    onSlideIndexChange=${setSummarySlideIndex}
                    onOpenFileFromSlide=${handleOpenFileFromSlide}
                    canOpenFileFromSlide=${canOpenFileFromSlide}
                    onClose=${() => setSlideShowOpen(false)}
                />
            `}
            `;
    }
    
    // Render the app
    render(html`<${App} />`, document.getElementById('app'));
}

// Initialize when DOM is ready
function startApp() {
    if (domReadyStartMs === null) {
        domReadyStartMs = getPerformanceNow();
    }
    initApp();
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', startApp);
} else {
    startApp();
}
