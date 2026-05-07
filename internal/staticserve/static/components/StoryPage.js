import { waitForPreact } from './utils.js';

function storyRefKey(ref) {
	if (!ref) return '';
	return JSON.stringify([
		ref.provider_id || ref.providerId || ref.ProviderID || '',
		ref.session_id || ref.sessionId || ref.SessionID || '',
	]);
}

function storyEventKey(event, fallbackIndex) {
	const message = event.message || event.Message;
	return JSON.stringify([
		event.index ?? event.Index ?? fallbackIndex,
		event.type || event.Type || '',
		event.timestamp || event.Timestamp || '',
		message?.message_id || message?.MessageID || '',
	]);
}

function storyToolKey(tool, fallbackIndex) {
	return JSON.stringify([
		tool.call_id || tool.CallID || '',
		tool.name || tool.Name || '',
		tool.phase || tool.Phase || '',
		fallbackIndex,
	]);
}

const INITIAL_RENDERABLE_EVENT_LIMIT = 200;
const RENDERABLE_EVENT_PAGE_SIZE = 200;
const STORY_RELATIVE_TIME_INTERVAL_MS = 30 * 1000;
const STORY_SESSION_PREVIEW_MAX_CHARS = 220;

const storyRelativeTimeFormatter = typeof Intl !== 'undefined' && Intl.RelativeTimeFormat
	? new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
	: null;

function parseStoryDate(value) {
	if (!value) return null;
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return null;
	return date;
}

function formatStoryAbsoluteDate(value) {
	const date = parseStoryDate(value);
	if (!date) return 'Unknown time';
	return date.toLocaleString();
}

function formatStoryRelativeDate(value, nowMs) {
	const date = parseStoryDate(value);
	if (!date) return 'Unknown time';
	const deltaSeconds = Math.round((date.getTime() - nowMs) / 1000);
	const absoluteSeconds = Math.abs(deltaSeconds);
	if (absoluteSeconds < 5) {
		return 'just now';
	}
	const ranges = [
		['year', 60 * 60 * 24 * 365],
		['month', 60 * 60 * 24 * 30],
		['week', 60 * 60 * 24 * 7],
		['day', 60 * 60 * 24],
		['hour', 60 * 60],
		['minute', 60],
		['second', 1],
	];
	for (const [unit, secondsPerUnit] of ranges) {
		if (absoluteSeconds >= secondsPerUnit || unit === 'second') {
			const roundedValue = Math.round(deltaSeconds / secondsPerUnit);
			if (storyRelativeTimeFormatter) {
				return storyRelativeTimeFormatter.format(roundedValue, unit);
			}
			const absoluteValue = Math.abs(roundedValue);
			const suffix = absoluteValue === 1 ? unit : `${unit}s`;
			return roundedValue < 0 ? `${absoluteValue} ${suffix} ago` : `in ${absoluteValue} ${suffix}`;
		}
	}
	return formatStoryAbsoluteDate(value);
}

function sortStoryEventsChronologically(events) {
	return [...events].sort((left, right) => {
		const leftIndex = left.index ?? left.Index;
		const rightIndex = right.index ?? right.Index;
		if (Number.isFinite(leftIndex) && Number.isFinite(rightIndex) && leftIndex !== rightIndex) {
			return leftIndex - rightIndex;
		}
		const leftDate = parseStoryDate(left.timestamp || left.Timestamp);
		const rightDate = parseStoryDate(right.timestamp || right.Timestamp);
		if (leftDate && rightDate && leftDate.getTime() !== rightDate.getTime()) {
			return leftDate.getTime() - rightDate.getTime();
		}
		if (Number.isFinite(leftIndex)) return -1;
		if (Number.isFinite(rightIndex)) return 1;
		return 0;
	});
}

function shortenStoryID(value, prefix = 8, suffix = 6) {
	if (!value) return '';
	if (value.length <= prefix + suffix + 1) return value;
	return `${value.slice(0, prefix)}...${value.slice(-suffix)}`;
}

function compactStoryText(value) {
	return String(value || '').replace(/\s+/g, ' ').trim();
}

function truncateStoryText(value, maxChars = STORY_SESSION_PREVIEW_MAX_CHARS) {
	const compactValue = compactStoryText(value);
	if (compactValue.length <= maxChars) {
		return compactValue;
	}
	const slice = compactValue.slice(0, maxChars + 1);
	const lastBoundary = Math.max(slice.lastIndexOf(' '), slice.lastIndexOf('.'), slice.lastIndexOf(','), slice.lastIndexOf(';'));
	const cutoff = lastBoundary >= Math.floor(maxChars * 0.6) ? lastBoundary : maxChars;
	return `${compactValue.slice(0, cutoff).trim()}...`;
}

function getRenderableStoryEvents(chat) {
	const events = chat?.events || chat?.Events || [];
	return sortStoryEventsChronologically(events.filter((event) => {
		if (event.message || event.Message) return true;
		const tools = event.tools || event.Tools || [];
		return Array.isArray(tools) && tools.length > 0;
	}));
}

function getStoryOpeningPrompt(chat, orderedEvents) {
	const metadataPrompt = chat?.draft_input || chat?.DraftInput || '';
	if (compactStoryText(metadataPrompt) !== '') {
		return metadataPrompt;
	}
	for (const event of orderedEvents) {
		const message = event.message || event.Message;
		const role = (message?.role || message?.Role || '').toLowerCase();
		const content = message?.content || message?.Content || '';
		if (role === 'user' && compactStoryText(content) !== '') {
			return content;
		}
	}
	return '';
}

function buildStoryTimelineEntries(events) {
	const entries = [];
	let pendingCluster = [];

	const flushCluster = () => {
		if (pendingCluster.length === 0) {
			return;
		}
		entries.push({
			kind: 'cluster',
			events: pendingCluster,
			firstEvent: pendingCluster[0],
			lastEvent: pendingCluster[pendingCluster.length - 1],
		});
		pendingCluster = [];
	};

	for (const event of events) {
		if (event.message || event.Message) {
			flushCluster();
			entries.push({ kind: 'message', event });
			continue;
		}
		pendingCluster.push(event);
	}

	flushCluster();
	return entries;
}

function storySpeakerLabel(message) {
	const role = (message?.role || message?.Role || '').toLowerCase();
	if (role === 'user') return 'You';
	if (role === 'assistant') return 'Copilot';
	if (!role) return 'Message';
	return role.charAt(0).toUpperCase() + role.slice(1);
}

function summarizeStoryTools(event) {
	const tools = event.tools || event.Tools || [];
	const toolCount = tools.length;
	const eventType = event.type || event.Type || 'internal step';
	if (toolCount === 0) return eventType;
	if (toolCount === 1) return `1 tool step`;
	return `${toolCount} tool steps`;
}

function summarizeStoryCluster(clusterEvents) {
	const typeCounts = new Map();
	const toolNames = new Set();
	let toolCount = 0;
	for (const event of clusterEvents) {
		const eventType = event.type || event.Type || 'internal step';
		typeCounts.set(eventType, (typeCounts.get(eventType) || 0) + 1);
		const tools = event.tools || event.Tools || [];
		toolCount += tools.length;
		for (const tool of tools) {
			const toolName = tool.name || tool.Name;
			if (toolName) {
				toolNames.add(toolName);
			}
		}
	}
	const topTypes = [...typeCounts.entries()]
		.sort((left, right) => right[1] - left[1])
		.slice(0, 2)
		.map(([type, count]) => `${count} ${type}`);
	const toolLabel = toolCount > 0 ? `${toolCount} tool step${toolCount !== 1 ? 's' : ''}` : `${clusterEvents.length} internal event${clusterEvents.length !== 1 ? 's' : ''}`;
	return {
		label: toolLabel,
		details: topTypes.join(' · '),
		toolNames: [...toolNames].slice(0, 3),
	};
}

export async function createStoryPage() {
	const { html, useEffect, useRef, useState } = await waitForPreact();

	return function StoryPage({
		commitContext,
		sessions,
		selectedSession,
		chat,
		loadingSessions,
		loadingChat,
		error,
		onSelectSession,
	}) {
		const renderableEvents = getRenderableStoryEvents(chat);
		const timelineEntries = buildStoryTimelineEntries(renderableEvents);
		const [visibleEventCount, setVisibleEventCount] = useState(INITIAL_RENDERABLE_EVENT_LIMIT);
		const [relativeNowMs, setRelativeNowMs] = useState(Date.now());
		const loadMoreRef = useRef(null);
		const selectedKey = storyRefKey(selectedSession);
		const selectedSessionSummary = sessions.find((session) => storyRefKey(session) === selectedKey) || null;
		const changedFiles = commitContext?.changed_files || [];
		const headCommit = commitContext?.head_commit || '';
		const visibleEntries = timelineEntries.slice(0, visibleEventCount);
		const hasMoreEvents = timelineEntries.length > visibleEventCount;
		const selectedChatTitle = selectedSessionSummary?.display_title || selectedSessionSummary?.DisplayTitle || chat?.display_title || chat?.DisplayTitle || 'Selected chat';
		const selectedDraftInput = selectedSessionSummary?.draft_input || selectedSessionSummary?.DraftInput || getStoryOpeningPrompt(chat, renderableEvents);
		const selectedSessionID = selectedSessionSummary?.session_id || selectedSessionSummary?.SessionID || chat?.session_id || chat?.SessionID || selectedSession?.session_id || selectedSession?.SessionID || '';

		useEffect(() => {
			setVisibleEventCount(INITIAL_RENDERABLE_EVENT_LIMIT);
		}, [chat?.session_id, chat?.SessionID]);

		useEffect(() => {
			const timerID = window.setInterval(() => {
				setRelativeNowMs(Date.now());
			}, STORY_RELATIVE_TIME_INTERVAL_MS);
			return () => window.clearInterval(timerID);
		}, []);

		useEffect(() => {
			if (!hasMoreEvents || !loadMoreRef.current || typeof IntersectionObserver === 'undefined') {
				return undefined;
			}
			const observer = new IntersectionObserver((entries) => {
				if (entries.some((entry) => entry.isIntersecting)) {
					setVisibleEventCount((count) => Math.min(count + RENDERABLE_EVENT_PAGE_SIZE, timelineEntries.length));
				}
			}, {
				rootMargin: '240px 0px',
				threshold: 0,
			});
			observer.observe(loadMoreRef.current);
			return () => observer.disconnect();
		}, [hasMoreEvents, timelineEntries.length]);

		return html`
			<section class="story-shell" aria-label="Story page">
				<div class="story-intro">
					<div class="story-intro-copy">
						<p class="story-eyebrow">Story</p>
						<h2 class="story-title">Chats captured for the current commit window</h2>
						<p class="story-subtitle">Priority goes to full sessions between the last commit and now. Diff-file overlap raises confidence, but sessions in the window remain visible even if file matching is weak.</p>
					</div>
					<div class="story-commit-context">
						<span class="story-context-label">Commit window</span>
						<strong title=${commitContext?.window_start && commitContext?.window_end ? `${formatStoryAbsoluteDate(commitContext.window_start)} to ${formatStoryAbsoluteDate(commitContext.window_end)}` : formatStoryAbsoluteDate(commitContext?.window_end)}>
							${commitContext?.window_start
								? `${formatStoryRelativeDate(commitContext.window_start, relativeNowMs)} to ${formatStoryRelativeDate(commitContext.window_end, relativeNowMs)}`
								: `No prior commit found, ending ${formatStoryRelativeDate(commitContext?.window_end, relativeNowMs)}`}
						</strong>
						${headCommit && html`<span class="story-secondary-meta" title=${headCommit}>HEAD ${shortenStoryID(headCommit, 7, 5)}</span>`}
						<span>${changedFiles.length} changed file${changedFiles.length !== 1 ? 's' : ''}</span>
						${changedFiles.length > 0 && html`
							<div class="story-commit-files">
								${changedFiles.slice(0, 8).map((filePath) => html`<span key=${filePath} class="story-commit-file">${filePath}</span>`)}
								${changedFiles.length > 8 && html`<span class="story-commit-file">+${changedFiles.length - 8} more</span>`}
							</div>
						`}
					</div>
				</div>

				${error && html`
					<div class="story-error-banner">${error}</div>
				`}

				<div class="story-layout">
					<div class="story-session-list" aria-label="Story sessions">
						<div class="story-pane-header">
							<h3>Commit-related chats</h3>
							<span>${loadingSessions ? 'Loading…' : `${sessions.length} found`}</span>
						</div>

						${loadingSessions && sessions.length === 0 && html`
							<div class="story-empty-state">Loading available chats…</div>
						`}

						${!loadingSessions && sessions.length === 0 && html`
							<div class="story-empty-state">No story sessions were found for the current providers.</div>
						`}

						${sessions.map((session) => {
							const sessionRef = { provider_id: session.provider_id, session_id: session.session_id };
							const isSelected = storyRefKey(sessionRef) === selectedKey;
							const title = session.display_title || session.preview || session.session_id;
							const sessionPreview = truncateStoryText(session.draft_input || session.preview || '');
							return html`
								<button
									key=${storyRefKey(sessionRef)}
									class="story-session-item ${isSelected ? 'active' : ''}"
									onClick=${() => onSelectSession(sessionRef)}
								>
									<div class="story-session-topline">
										<div class="story-session-topline-left">
											<span class="story-session-provider">${session.provider_id}</span>
											${session.workspace_id && html`<span class="story-secondary-meta">${session.session_scope || 'workspace'} · ${session.workspace_id}</span>`}
										</div>
										<time class="story-session-updated" title=${formatStoryAbsoluteDate(session.updated_at)} dateTime=${session.updated_at || ''}>
											${formatStoryRelativeDate(session.updated_at, relativeNowMs)}
										</time>
									</div>
									<div class="story-session-title">${title}</div>
									${sessionPreview && html`<p class="story-session-preview">${sessionPreview}</p>`}
									<div class="story-session-footnote">
										<div class="story-session-badge-row">
										${session.recommended && html`<span class="story-session-badge">Suggested</span>`}
										${session.within_commit_window && html`<span class="story-session-badge window">In window</span>`}
										</div>
										<span class="story-session-id" title=${session.session_id}>${shortenStoryID(session.session_id, 6, 5)}</span>
									</div>
									${Array.isArray(session.matched_files) && session.matched_files.length > 0 && html`
										<div class="story-match-reasons">
											${session.matched_files.slice(0, 4).map((filePath) => html`<span key=${filePath} class="story-match-pill">${filePath}</span>`)}
										</div>
									`}
									${Array.isArray(session.match_reasons) && session.match_reasons.length > 0 && html`
										<div class="story-match-reasons">
											${session.match_reasons.map((reason) => html`<span key=${reason} class="story-match-pill">${reason}</span>`)}
										</div>
									`}
								</button>
							`;
						})}
					</div>

					<div class="story-preview-pane" aria-label="Story preview">
						<div class="story-pane-header">
							<h3>Selected chat</h3>
							${selectedSession && html`<span class="story-secondary-meta">${selectedSession.provider_id}</span>`}
						</div>

						${loadingChat && html`
							<div class="story-empty-state">Loading selected chat…</div>
						`}

						${!loadingChat && !chat && html`
							<div class="story-empty-state">Choose a chat to inspect its full interaction flow.</div>
						`}

						${!loadingChat && chat && html`
							<div class="story-chat-summary">
								<h4 class="story-chat-title">${selectedChatTitle}</h4>
								${selectedDraftInput && html`
									<p class="story-chat-prompt">${selectedDraftInput}</p>
								`}
								<div class="story-chat-headline-meta">
									<time class="story-chat-updated" title=${formatStoryAbsoluteDate(chat.updated_at || chat.UpdatedAt)} dateTime=${chat.updated_at || chat.UpdatedAt || ''}>
										Updated ${formatStoryRelativeDate(chat.updated_at || chat.UpdatedAt, relativeNowMs)}
									</time>
									${selectedSessionID && html`<span class="story-secondary-meta" title=${selectedSessionID}>${shortenStoryID(selectedSessionID)}</span>`}
								</div>
								<details class="story-details-toggle">
									<summary>Conversation details</summary>
									<div class="story-chat-summary-grid">
										<div>
											<span class="story-context-label">Provider</span>
											<strong>${chat.provider_id || chat.ProviderID}</strong>
										</div>
										<div>
											<span class="story-context-label">Renderable events</span>
											<strong>${renderableEvents.length}</strong>
										</div>
										<div>
											<span class="story-context-label">Scope</span>
											<strong>${chat.session_scope || chat.SessionScope || 'workspace'}</strong>
										</div>
										<div>
											<span class="story-context-label">Started</span>
											<strong title=${formatStoryAbsoluteDate(chat.started_at || chat.StartedAt)}>${formatStoryRelativeDate(chat.started_at || chat.StartedAt, relativeNowMs)}</strong>
										</div>
										<div>
											<span class="story-context-label">Updated</span>
											<strong title=${formatStoryAbsoluteDate(chat.updated_at || chat.UpdatedAt)}>${formatStoryRelativeDate(chat.updated_at || chat.UpdatedAt, relativeNowMs)}</strong>
										</div>
									</div>
								</details>
								${Array.isArray(chat.warnings || chat.Warnings) && (chat.warnings || chat.Warnings).length > 0 && html`
									<details class="story-details-toggle story-warning-toggle">
										<summary>${(chat.warnings || chat.Warnings).length} warning${(chat.warnings || chat.Warnings).length !== 1 ? 's' : ''}</summary>
										<div class="story-warning-list">
											${(chat.warnings || chat.Warnings).map((warning, warningIndex) => html`<div key=${JSON.stringify([warningIndex, warning])} class="story-warning-item">${warning}</div>`)}
										</div>
									</details>
								`}
							</div>

							<div class="story-event-list" aria-label="Conversation timeline">
								${renderableEvents.length > INITIAL_RENDERABLE_EVENT_LIMIT && html`
									<div class="story-session-meta">Showing ${Math.min(visibleEventCount, timelineEntries.length)} of ${timelineEntries.length} timeline entries</div>
								`}
								${renderableEvents.length === 0 && html`
									<div class="story-empty-state">This chat does not have renderable message events.</div>
								`}
								${visibleEntries.map((entry, entryIndex) => {
									if (entry.kind === 'cluster') {
										const clusterSummary = summarizeStoryCluster(entry.events);
										const clusterTimestamp = entry.lastEvent?.timestamp || entry.lastEvent?.Timestamp || entry.firstEvent?.timestamp || entry.firstEvent?.Timestamp;
										return html`
											<details key=${storyEventKey(entry.firstEvent, entryIndex)} class="story-details-toggle story-cluster-event">
												<summary>
													<div class="story-cluster-summary-copy">
														<span class="story-cluster-label">${clusterSummary.label}</span>
														${clusterSummary.details && html`<span class="story-secondary-meta">${clusterSummary.details}</span>`}
														${clusterSummary.toolNames.length > 0 && html`<span class="story-secondary-meta">${clusterSummary.toolNames.join(' · ')}</span>`}
													</div>
													<time class="story-event-time" title=${formatStoryAbsoluteDate(clusterTimestamp)} dateTime=${clusterTimestamp || ''}>${formatStoryRelativeDate(clusterTimestamp, relativeNowMs)}</time>
												</summary>
												<div class="story-cluster-list">
													${entry.events.map((event, clusterIndex) => {
														const tools = event.tools || event.Tools || [];
														const eventTimestamp = event.timestamp || event.Timestamp;
														return html`
															<div key=${storyEventKey(event, clusterIndex)} class="story-cluster-item">
																<div class="story-cluster-item-head">
																	<span>${event.type || event.Type || 'internal step'}</span>
																	<time class="story-event-time" title=${formatStoryAbsoluteDate(eventTimestamp)} dateTime=${eventTimestamp || ''}>${formatStoryRelativeDate(eventTimestamp, relativeNowMs)}</time>
																</div>
																${Array.isArray(tools) && tools.length > 0 && html`
																	<div class="story-tool-list story-tool-list-compact">
																		${tools.map((tool, toolIndex) => html`
																			<div key=${storyToolKey(tool, toolIndex)} class="story-tool-item">
																				<div class="story-tool-head">
																					<strong>${tool.name || tool.Name || 'tool'}</strong>
																					<span>${tool.phase || tool.Phase || ''}</span>
																				</div>
																				${tool.arguments || tool.Arguments ? html`<pre class="story-tool-args">${tool.arguments || tool.Arguments}</pre>` : null}
																			</div>
																		`)}
																	</div>
																`}
															</div>
														`;
													})}
												</div>
											</details>
										`;
									}

									const event = entry.event;
									const message = event.message || event.Message;
									const tools = event.tools || event.Tools || [];
									const eventTimestamp = event.timestamp || event.Timestamp;
									return html`
										<div key=${storyEventKey(event, entryIndex)} class="story-event-item role-${(message.role || message.Role || 'message').toLowerCase()}">
											<article class="story-message-row">
												<div class="story-message-head">
													<div class="story-message-speaker-group">
														<span class="story-event-role role-${(message.role || message.Role || 'message').toLowerCase()}">${storySpeakerLabel(message)}</span>
														<span class="story-secondary-meta">${event.type || event.Type || 'message'}</span>
													</div>
													<time class="story-event-time" title=${formatStoryAbsoluteDate(eventTimestamp)} dateTime=${eventTimestamp || ''}>${formatStoryRelativeDate(eventTimestamp, relativeNowMs)}</time>
												</div>
												<pre class="story-message-content">${message.content || message.Content || ''}</pre>
												${Array.isArray(tools) && tools.length > 0 && html`
													<details class="story-details-toggle story-tool-toggle">
														<summary>${summarizeStoryTools(event)}</summary>
														<div class="story-tool-list">
															${tools.map((tool, toolIndex) => html`
																<div key=${storyToolKey(tool, toolIndex)} class="story-tool-item">
																	<div class="story-tool-head">
																		<strong>${tool.name || tool.Name || 'tool'}</strong>
																		<span>${tool.phase || tool.Phase || ''}</span>
																	</div>
																	${tool.arguments || tool.Arguments ? html`<pre class="story-tool-args">${tool.arguments || tool.Arguments}</pre>` : null}
																</div>
															`)}
														</div>
													</details>
												`}
											</article>
										</div>
									`;
								})}
								${hasMoreEvents && html`
									<div ref=${loadMoreRef} class="story-load-more-sentinel" aria-hidden="true"></div>
									<button class="btn btn-secondary story-load-more-btn" onClick=${() => setVisibleEventCount((count) => count + RENDERABLE_EVENT_PAGE_SIZE)}>
										Show ${Math.min(RENDERABLE_EVENT_PAGE_SIZE, timelineEntries.length - visibleEventCount)} more timeline entries
									</button>
								`}
							</div>
						`}
					</div>
				</div>
			</section>
		`;
	};
}

let StoryPageComponent = null;

export async function getStoryPage() {
	if (!StoryPageComponent) {
		StoryPageComponent = await createStoryPage();
	}
	return StoryPageComponent;
}