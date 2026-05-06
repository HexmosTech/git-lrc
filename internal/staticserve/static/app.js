// LiveReview App - Main Entry Point
// Fetches data from /api/review and updates reactively

import { waitForPreact, filePathToId, transformEvent, getBadgeClass, formatIssueForCopy, getCommentVisibilityKey } from './components/utils.js';
import { getHeader } from './components/Header.js';
import { getSidebar } from './components/Sidebar.js';
import { getSummary } from './components/Summary.js';
import { getStats } from './components/Stats.js';
import { getPrecommitBar } from './components/PrecommitBar.js';
import { getFileBlock } from './components/FileBlock.js';
import { getEventLog } from './components/EventLog.js';
import { getSeverityFilter } from './components/SeverityFilter.js';
import { getToolbar } from './components/Toolbar.js';
import { getCommentNav } from './components/CommentNav.js';
import { UsageBanner } from './components/UsageBanner.js';

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
                BadgeClass: getBadgeClass(comment.severity || comment.Severity || 'info'),
                Category: comment.category || comment.Category || '',
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
    const SeverityFilter = await getSeverityFilter();
    const Toolbar = await getToolbar();
    const CommentNav = await getCommentNav();
    
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
        const [visibleSeverities, setVisibleSeverities] = useState(new Set(['critical', 'error', 'warning', 'info']));
        const [events, setEvents] = useState([]);
        const [newEventCount, setNewEventCount] = useState(0);
        const [isTailing, setIsTailing] = useState(false);
        const [hiddenCommentKeys, setHiddenCommentKeys] = useState(new Set());
        const [copyFeedback, setCopyFeedback] = useState({ status: 'idle', message: '' });
        const [handoffModal, setHandoffModal] = useState({ isOpen: false, type: '', message: '' });
        
        const pollingRef = useRef(null);
        const eventsPollingRef = useRef(null);
        const eventsListRef = useRef(null);
        const copyFeedbackTimerRef = useRef(null);
        const [logsCopied, setLogsCopied] = useState(false);
        
        // Fetch review data from API
        const fetchReviewData = useCallback(async () => {
            try {
                const response = await fetch('/api/review');
                if (!response.ok) {
                    throw new Error(`Failed to fetch review data: ${response.status}`);
                }
                const data = await response.json();
                
                // Convert files to UI format
                const uiFiles = convertFilesToUIFormat(data.files);
                
                // Calculate actual comment count from files (don't trust API counter)
                const actualCommentCount = countCommentsFromFiles(data.files);
                
                setReviewData(prev => {
                    // Auto-expand files with comments
                    // On first load: expand all files with comments
                    // On updates: also expand any NEW files that have comments
                    if (!prev) {
                        // First load - expand all files with comments
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
                        // Subsequent updates - expand any new files with comments
                        const prevFileIds = new Set((prev.Files || []).map(f => f.ID));
                        const newFilesWithComments = uiFiles.filter(
                            file => file.HasComments && !prevFileIds.has(file.ID)
                        );
                        if (newFilesWithComments.length > 0) {
                            setExpandedFiles(prevExpanded => {
                                const next = new Set(prevExpanded);
                                newFilesWithComments.forEach(file => next.add(file.ID));
                                return next;
                            });
                        }
                    }
                    
                    return {
                        ...data,
                        Files: uiFiles,
                        TotalFiles: uiFiles.length,
                        TotalComments: actualCommentCount  // Derived from actual file comments
                    };
                });
                
                setLoading(false);
                return data;
            } catch (err) {
                console.error('Error fetching review data:', err);
                setError(err.message);
                setLoading(false);
                return null;
            }
        }, []);
        
        // Fetch events for the event log
        const fetchEvents = useCallback(async (reviewID) => {
            if (!reviewID) return;
            
            try {
                const response = await fetch(`/api/v1/diff-review/${reviewID}/events?limit=1000`);
                if (!response.ok) return;
                
                const data = await response.json();
                const backendEvents = data.events || [];
                const transformedEvents = backendEvents.map(transformEvent);
                
                setEvents(prev => {
                    if (transformedEvents.length > prev.length) {
                        const addedCount = transformedEvents.length - prev.length;
                        if (activeTab !== 'events') {
                            setNewEventCount(count => count + addedCount);
                        }
                    }
                    return transformedEvents;
                });
            } catch (err) {
                console.error('Error fetching events:', err);
            }
        }, [activeTab]);
        
        // Initial load and polling setup
        useEffect(() => {
            // Initial fetch
            fetchReviewData().then(data => {
                if (data?.reviewID) {
                    fetchEvents(data.reviewID);
                }
            });
            
            // Poll for updates every 2 seconds
            pollingRef.current = setInterval(async () => {
                const data = await fetchReviewData();
                if (data?.reviewID) {
                    fetchEvents(data.reviewID);
                }
                
                // Stop polling when review is complete
                if (data?.status === 'completed' || data?.status === 'failed') {
                    if (pollingRef.current) {
                        clearInterval(pollingRef.current);
                        pollingRef.current = null;
                    }
                }
            }, 2000);
            
            // Cleanup
            return () => {
                if (pollingRef.current) {
                    clearInterval(pollingRef.current);
                }
            };
        }, [fetchReviewData, fetchEvents]);
        
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
        const handleFileClick = useCallback((fileId) => {
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
                    const fileRect = fileEl.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + fileRect.top - mainContentRect.top - headerHeight - 10;
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                }
            }, 100);
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
                    const header = document.querySelector('.header');
                    const headerHeight = header ? header.offsetHeight : 60;
                    const commentRect = comment.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + commentRect.top - mainContentRect.top - headerHeight - 20;
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                    
                    comment.classList.add('highlight');
                    setTimeout(() => comment.classList.remove('highlight'), 1500);
                }
            }, 100);
        }, []);
        
        // Tab change
        const handleTabChange = useCallback((tab) => {
            setActiveTab(tab);
            if (tab === 'events') {
                setNewEventCount(0);
            }
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

        const handleSendToAgent = useCallback(async () => {
            const filteredFiles = (reviewData.files || reviewData.Files || []).map(file => {
                const filePath = file.file_path || file.filePath || file.FilePath;
                const newComments = (file.comments || file.Comments || []).filter(c => {
                    const sev = (c.severity || c.Severity || '').toLowerCase();
                    if (!visibleSeverities.has(sev)) return false;
                    const key = getCommentVisibilityKey(filePath, c);
                    return !hiddenCommentKeys.has(key);
                });
                return { ...file, comments: newComments, Comments: newComments };
            }).filter(file => file.comments.length > 0);
            
            if (filteredFiles.length === 0) {
                setHandoffModal({ 
                    isOpen: true, 
                    type: 'error', 
                    message: "No visible comments to send to the AI agent. Please show some comments first." 
                });
                return;
            }
            
            const payload = {
                ...reviewData,
                files: filteredFiles,
                Files: filteredFiles,
                summary: "AI Agent Handoff generated for visible issues.",
                status: "completed"
            };
            
            try {
                const response = await fetch('/handoff', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (!response.ok) throw new Error("Handoff failed");
                setHandoffModal({ 
                    isOpen: true, 
                    type: 'success', 
                    message: "Claude Code is now starting in your terminal! You can safely close this browser window." 
                });
            } catch (e) {
                setHandoffModal({ 
                    isOpen: true, 
                    type: 'error', 
                    message: "Failed to send to agent: " + e.message 
                });
            }
        }, [reviewData, visibleSeverities, hiddenCommentKeys]);

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

        useEffect(() => {
            return () => {
                if (copyFeedbackTimerRef.current) {
                    clearTimeout(copyFeedbackTimerRef.current);
                    copyFeedbackTimerRef.current = null;
                }
            };
        }, []);
        
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
        
        // Loading state
        if (loading && !reviewData) {
            return html`
                <div class="loading-screen">
                    <div class="loading-content">
                        <div class="loading-logo">
                            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                                <circle cx="12" cy="12" r="10" />
                                <path d="M12 6v6l4 2" stroke-linecap="round" />
                            </svg>
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
                            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                                <circle cx="12" cy="12" r="10" />
                                <path d="M15 9l-6 6M9 9l6 6" stroke-linecap="round" />
                            </svg>
                        </div>
                        <h1 class="loading-title">LiveReview</h1>
                        <h2 class="loading-error-title">Error Loading Review</h2>
                        <p class="loading-error-text">${error}</p>
                    </div>
                </div>
            `;
        }
        
        const status = reviewData?.status || 'in_progress';
        const showLoader = status === 'in_progress';
        const summary = reviewData?.summary || '';
        const files = reviewData?.Files || [];
        
        // Toggle severity visibility
        const toggleSeverity = useCallback((severity) => {
            setVisibleSeverities(prev => {
                const next = new Set(prev);
                if (next.has(severity)) {
                    next.delete(severity);
                } else {
                    next.add(severity);
                }
                return next;
            });
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
                                const sev = (comment.Severity || '').toLowerCase();
                                if (!visibleSeverities.has(sev)) return;
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
        }, [files, visibleSeverities, hiddenCommentKeys, showCopyFeedback]);
        
        // Build flat ordered list of VISIBLE comments for navigation
        const allComments = [];
        const commentIds = [];
        files.forEach(file => {
            const fileId = file.ID || filePathToId(file.FilePath);
            file.Hunks.forEach(hunk => {
                hunk.Lines.forEach(line => {
                    if (line.IsComment && line.Comments) {
                        line.Comments.forEach((comment, commentIdx) => {
                            const sev = (comment.Severity || '').toLowerCase();
                            if (!visibleSeverities.has(sev)) return;
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
        
        // Calculate totalComments from actual files - single source of truth
        const totalComments = files.reduce((sum, file) => sum + (file.CommentCount || 0), 0);
        
        // Calculate visible comments for the agent button
        let totalVisibleComments = 0;
        files.forEach(file => {
            if (!file.HasComments) return;
            file.Hunks.forEach(hunk => {
                hunk.Lines.forEach(line => {
                    if (line.IsComment && line.Comments) {
                        line.Comments.forEach((comment) => {
                            const sev = (comment.Severity || '').toLowerCase();
                            if (!visibleSeverities.has(sev)) return;
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
                        <span class="status-icon">❌</span>
                        <span>Review completed with errors</span>
                    </div>
                `;
            }
            if (status === 'completed') {
                return html`
                    <div class="status-container success">
                        <span class="status-icon">✅</span>
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
                visibleSeverities=${visibleSeverities}
            />
            <div class="main-content">
                <div class="container">
                    <${Header} 
                        generatedTime=${reviewData?.generatedTime || reviewData?.GeneratedTime}
                        friendlyName=${reviewData?.friendlyName || reviewData?.FriendlyName}
                    />
                    
                    ${showLoader && html`
                        <div class="loader-container">
                            <div class="loader-content">
                                <div class="spinner"></div>
                                <span class="loader-message">Review in progress...</span>
                            </div>
                        </div>
                    `}
                    
                    ${getStatusDisplay()}
                    
                    <${UsageBanner} endpoint="/api/runtime/usage-chip" />
                    
                    ${summary && summary.trim() && status !== 'in_progress' && html`
                        <${Summary} 
                            markdown=${summary}
                            status=${status}
                            errorSummary=${reviewData?.errorSummary || ''}
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
                        <${SeverityFilter}
                            files=${files}
                            visibleSeverities=${visibleSeverities}
                            onToggleSeverity=${toggleSeverity}
                            onCopyVisibleIssues=${handleCopyVisibleIssues}
                            hiddenCommentKeys=${hiddenCommentKeys}
                            copyFeedbackStatus=${copyFeedback.status}
                            copyFeedbackMessage=${copyFeedback.message}
                            onSendToAgent=${handleSendToAgent}
                            visibleCount=${totalVisibleComments}
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
                                    visibleSeverities=${visibleSeverities}
                                    hiddenCommentKeys=${hiddenCommentKeys}
                                    onToggleCommentVisibility=${toggleCommentVisibility}
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
                    
                    ${handoffModal.isOpen && html`
                        <div class="modal-overlay" style="position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); z-index: 9999; display: flex; align-items: center; justify-content: center; backdrop-filter: blur(4px);">
                            <div class="modal-content" style="background: var(--bg-card); padding: 32px; border-radius: 12px; max-width: 400px; width: 90%; border: 1px solid var(--border-color); box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5); text-align: center;">
                                ${handoffModal.type === 'success' 
                                    ? html`
                                        <div style="margin-bottom: 16px; color: #8b5cf6;">
                                            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                                <rect x="2" y="3" width="20" height="18" rx="2" ry="2"></rect>
                                                <polyline points="6 8 10 12 6 16"></polyline>
                                                <line x1="14" y1="16" x2="18" y2="16"></line>
                                            </svg>
                                        </div>
                                    `
                                    : html`<div style="font-size: 48px; margin-bottom: 16px;">⚠️</div>`
                                }
                                <h3 style="margin: 0 0 12px 0; font-size: 20px; color: var(--text-primary);">
                                    ${handoffModal.type === 'success' ? 'Check Your Terminal' : 'Notice'}
                                </h3>
                                <p style="margin: 0 0 24px 0; color: var(--text-secondary); line-height: 1.5;">
                                    ${handoffModal.message}
                                </p>
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
            />
        `;
    }
    
    // Render the app
    render(html`<${App} />`, document.getElementById('app'));
}

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initApp);
} else {
    initApp();
}
