/**
 * Markdown to slideshow parser.
 * Produces compact, presentation-friendly slides from review summaries.
 */

const MIN_SLIDE_SECONDS = 5;
const MAX_SLIDE_SECONDS = 12;
const SHORT_LIST_CHUNK_SIZE = 2;

const SLIDE_COLORS = [
  {
    surface: '#1f2733',
    accent: '#4f8cff',
    title: '#eaf1ff',
    text: '#d7e2ff',
    name: 'blue'
  },
  {
    surface: '#1f2c29',
    accent: '#38b28a',
    title: '#e8fff7',
    text: '#c8f5e8',
    name: 'mint'
  },
  {
    surface: '#26243a',
    accent: '#9a7bff',
    title: '#f0ecff',
    text: '#ddd4ff',
    name: 'violet'
  },
  {
    surface: '#33222c',
    accent: '#ff6b94',
    title: '#ffeaf2',
    text: '#ffd1e2',
    name: 'rose'
  },
  {
    surface: '#30271b',
    accent: '#f5a524',
    title: '#fff4de',
    text: '#ffe2b0',
    name: 'amber'
  }
];

const SENTENCE_PROTECTIONS = [
  /\b(?:e\.g|i\.e|etc|vs|Mr|Mrs|Ms|Dr|Prof|Sr|Jr|St|Inc|Ltd|No)\./g,
  /\b(?:Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Sept|Oct|Nov|Dec)\./g,
  /\.\.\./g,
  /\b\d+\.\d+\b/g,
  /https?:\/\/\S+/g,
  /`[^`]+`/g
];

function countWords(text) {
  const trimmed = (text || '').trim();
  return trimmed ? trimmed.split(/\s+/).length : 0;
}

function extractPlainText(html) {
  const raw = html || '';
  if (typeof document === 'undefined') {
    return raw.replace(/<[^>]+>/g, ' ');
  }

  const container = document.createElement('div');
  container.innerHTML = raw;
  return container.textContent || '';
}

function estimateReadTimeSeconds(text, title) {
  const words = countWords(`${title || ''} ${extractPlainText(text || '')}`);
  if (!words) {
    return MIN_SLIDE_SECONDS;
  }

  const estimated = Math.round(3.5 + (words / 3.2));
  return Math.max(MIN_SLIDE_SECONDS, Math.min(MAX_SLIDE_SECONDS, estimated));
}

function formatTime(seconds) {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const secs = seconds % 60;
  if (secs === 0) return `${minutes}m`;
  return `${minutes}m ${secs}s`;
}

function cleanContent(text) {
  return (text || '').trim();
}

function protectSentenceTokens(text) {
  const replacements = [];
  let protectedText = text;

  SENTENCE_PROTECTIONS.forEach((pattern) => {
    protectedText = protectedText.replace(pattern, (match) => {
      const token = `__SLIDE_TOKEN_${replacements.length}__`;
      replacements.push(match);
      return token;
    });
  });

  return { protectedText, replacements };
}

function restoreSentenceTokens(text, replacements) {
  return replacements.reduce(
    (acc, value, index) => acc.replaceAll(`__SLIDE_TOKEN_${index}__`, value),
    text
  );
}

function convertMarkdownToHtml(markdown) {
  if (typeof marked === 'undefined') {
    return '';
  }

  return marked.parse(markdown || '', { mangle: false, headerIds: false, gfm: true, breaks: true });
}

function createParsedHtmlRoot(markdown) {
  if (typeof DOMParser === 'undefined') {
    return null;
  }

  const html = convertMarkdownToHtml(markdown);
  const parsed = new DOMParser().parseFromString(`<div id="slideshow-parser-root">${html}</div>`, 'text/html');
  return parsed.getElementById('slideshow-parser-root');
}

function getDirectTextContent(node) {
  return (node && node.textContent ? node.textContent : '').replace(/\s+/g, ' ').trim();
}

function serializeNode(node) {
  if (!node) {
    return '';
  }

  if (node.outerHTML) {
    return node.outerHTML;
  }

  if (typeof XMLSerializer !== 'undefined') {
    return new XMLSerializer().serializeToString(node);
  }

  return node.textContent || '';
}

function collectTextNodeRanges(root) {
  const textNodes = [];
  const walker = document.createTreeWalker(root, 4);
  let text = '';

  while (walker.nextNode()) {
    const node = walker.currentNode;
    const value = node.nodeValue || '';
    if (!value) {
      continue;
    }

    const start = text.length;
    text += value;
    textNodes.push({ node, start, end: text.length });
  }

  return { text, textNodes };
}

function findTextPosition(textNodes, offset) {
  if (!textNodes.length) {
    return null;
  }

  if (offset <= 0) {
    return { node: textNodes[0].node, offset: 0 };
  }

  const last = textNodes[textNodes.length - 1];
  if (offset >= last.end) {
    return { node: last.node, offset: last.node.nodeValue ? last.node.nodeValue.length : 0 };
  }

  for (const entry of textNodes) {
    if (offset >= entry.start && offset <= entry.end) {
      return { node: entry.node, offset: offset - entry.start };
    }
  }

  return { node: last.node, offset: last.node.nodeValue ? last.node.nodeValue.length : 0 };
}

function getSentenceRanges(text) {
  if (!text || !text.trim()) {
    return [];
  }

  if (typeof Intl !== 'undefined' && typeof Intl.Segmenter === 'function') {
    const segmenter = new Intl.Segmenter(undefined, { granularity: 'sentence' });
    return Array.from(segmenter.segment(text))
      .map(segment => ({ start: segment.index, end: segment.index + segment.segment.length }))
      .filter(range => text.slice(range.start, range.end).trim());
  }

  const { protectedText, replacements } = protectSentenceTokens(text);
  const parts = protectedText
    .split(/(?<=[.!?])\s+(?=(?:["'(\[])?[A-Z0-9])/)
    .map(part => restoreSentenceTokens(part, replacements))
    .filter(Boolean);

  if (!parts.length) {
    return [{ start: 0, end: text.length }];
  }

  const ranges = [];
  let cursor = 0;

  for (const part of parts) {
    const start = text.indexOf(part, cursor);
    if (start === -1) {
      const fallbackStart = cursor;
      const fallbackEnd = Math.min(text.length, fallbackStart + part.length);
      ranges.push({ start: fallbackStart, end: fallbackEnd });
      cursor = fallbackEnd;
      continue;
    }

    const end = start + part.length;
    ranges.push({ start, end });
    cursor = end;
  }

  return ranges.length ? ranges : [{ start: 0, end: text.length }];
}

function splitParagraphNode(paragraphNode) {
  if (paragraphNode.nodeType === Node.TEXT_NODE) {
    const wrapper = document.createElement('p');
    wrapper.textContent = paragraphNode.nodeValue || '';
    return splitParagraphNode(wrapper);
  }

  const { text, textNodes } = collectTextNodeRanges(paragraphNode);
  const ranges = getSentenceRanges(text);

  if (ranges.length <= 1) {
    return [serializeNode(paragraphNode)];
  }

  const fragments = [];

  ranges.forEach(rangeInfo => {
    const startPosition = findTextPosition(textNodes, rangeInfo.start);
    const endPosition = findTextPosition(textNodes, rangeInfo.end);
    if (!startPosition || !endPosition) {
      return;
    }

    const range = document.createRange();
    range.setStart(startPosition.node, startPosition.offset);
    range.setEnd(endPosition.node, endPosition.offset);

    const wrapper = paragraphNode.cloneNode(false);
    wrapper.appendChild(range.cloneContents());
    const serialized = serializeNode(wrapper);
    if (serialized && wrapper.textContent.trim()) {
      fragments.push(serialized);
    }
  });

  return fragments.length ? fragments : [serializeNode(paragraphNode)];
}

function chunkListItems(items) {
  if (items.length <= SHORT_LIST_CHUNK_SIZE) {
    return [items];
  }

  const chunks = [];
  let index = 0;
  while (index < items.length) {
    const remaining = items.length - index;
    const chunkSize = remaining === 3 ? 1 : SHORT_LIST_CHUNK_SIZE;
    chunks.push(items.slice(index, index + chunkSize));
    index += chunkSize;
  }
  return chunks;
}

function cloneListChunk(listNode, items) {
  const clone = listNode.cloneNode(false);
  items.forEach(item => clone.appendChild(item.cloneNode(true)));
  return serializeNode(clone);
}

function createSlide(content, color, options = {}) {
  const title = options.title || '';
  const readTime = estimateReadTimeSeconds(cleanContent(content), title);

  return {
    title,
    content: cleanContent(content),
    kind: options.kind || 'sentence',
    readTime,
    readTimeFormatted: formatTime(readTime),
    color,
    isMarkdown: false
  };
}

export function parseMarkdownToSlides(markdown) {
  if (!markdown || !markdown.trim()) {
    return [];
  }

  if (typeof document === 'undefined' || typeof DOMParser === 'undefined') {
    return [];
  }

  const root = createParsedHtmlRoot(markdown);
  if (!root) {
    return [];
  }

  const blocks = Array.from(root.childNodes).filter(node => {
    if (node.nodeType === Node.TEXT_NODE) {
      return (node.nodeValue || '').trim().length > 0;
    }

    return node.nodeType === Node.ELEMENT_NODE;
  });

  const slides = [];
  let colorIndex = 0;
  let sectionTitle = '';

  const nextColor = () => {
    const color = SLIDE_COLORS[colorIndex % SLIDE_COLORS.length];
    colorIndex += 1;
    return color;
  };

  blocks.forEach((block, blockIndex) => {
    if (block.nodeType === Node.TEXT_NODE) {
      const text = (block.nodeValue || '').trim();
      if (!text) {
        return;
      }
      splitParagraphNode(block).forEach(sentenceHtml => {
        slides.push(createSlide(sentenceHtml, nextColor(), { title: sectionTitle, kind: 'sentence' }));
      });
      return;
    }

    const element = block;
    const tagName = element.tagName;

    if (/^H[1-6]$/.test(tagName)) {
      const headingText = getDirectTextContent(element);
      if (!headingText) {
        return;
      }

      if (tagName === 'H1' && slides.length === 0 && blockIndex === 0) {
        slides.push(createSlide('', nextColor(), { title: headingText, kind: 'intro' }));
        sectionTitle = '';
        return;
      }

      sectionTitle = headingText;
      return;
    }

    if (tagName === 'P') {
      splitParagraphNode(element).forEach(sentenceHtml => {
        slides.push(createSlide(sentenceHtml, nextColor(), { title: sectionTitle, kind: 'sentence' }));
      });
      return;
    }

    if (tagName === 'UL' || tagName === 'OL') {
      const items = Array.from(element.children).filter(child => child.tagName === 'LI');
      chunkListItems(items).forEach(chunk => {
        slides.push(createSlide(cloneListChunk(element, chunk), nextColor(), { title: sectionTitle, kind: 'list' }));
      });
      return;
    }

    if (tagName === 'PRE' || tagName === 'BLOCKQUOTE' || tagName === 'TABLE' || tagName === 'HR') {
      slides.push(createSlide(serializeNode(element), nextColor(), { title: sectionTitle, kind: tagName === 'PRE' ? 'code' : 'block' }));
      return;
    }

    if (tagName === 'DIV') {
      const childElements = Array.from(element.children);
      if (childElements.length === 1 && childElements[0].tagName === 'PRE') {
        slides.push(createSlide(serializeNode(childElements[0]), nextColor(), { title: sectionTitle, kind: 'code' }));
        return;
      }
    }

    const textContent = getDirectTextContent(element);
    if (textContent) {
      splitParagraphNode(element).forEach(sentenceHtml => {
        slides.push(createSlide(sentenceHtml, nextColor(), { title: sectionTitle, kind: 'sentence' }));
      });
    }
  });

  const totalReadTime = slides.reduce((sum, slide) => sum + slide.readTime, 0);
  slides.forEach((slide, index) => {
    slide.slideNumber = index + 1;
    slide.totalSlides = slides.length;
    slide.totalReadTime = totalReadTime;
  });

  return slides;
}

export function calculateTotalReadTime(slides) {
  return slides.reduce((sum, slide) => sum + slide.readTime, 0);
}

export function formatTotalReadTime(slides) {
  return formatTime(calculateTotalReadTime(slides));
}

export function getRemainingReadTime(slides, currentSlideIndex) {
  if (!slides || currentSlideIndex >= slides.length) {
    return 0;
  }

  return slides.slice(currentSlideIndex).reduce((sum, slide) => sum + slide.readTime, 0);
}

export function formatRemainingTime(slides, currentSlideIndex) {
  return formatTime(getRemainingReadTime(slides, currentSlideIndex));
}
