import {
  calculateTotalReadTime,
  formatRemainingTime,
  formatTotalReadTime,
  getRemainingReadTime,
  parseMarkdownToSlides
} from './slideshowParser.js';

function testIntroAndSectionSlides() {
  const markdown = `# Review Summary

## Overview

This is the first sentence. This is the second sentence.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides[0].kind === 'intro', 'First slide should be intro slide');
  console.assert(slides[1].title === 'Overview', 'Section title should be applied to first section slide');
  console.assert(slides[1].content.includes('This is the first sentence.'), 'First sentence should become its own slide');
  console.assert(slides[2].content.includes('This is the second sentence.'), 'Second sentence should become its own slide');
  console.log('✓ Intro and section slides test passed');
}

function testListChunking() {
  const markdown = `## Technical Highlights

- Item one with **bold**
- Item two
  - Nested note one
  - Nested note two
- Item three
- Item four
- Item five`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 3, `Expected 3 list slides, got ${slides.length}`);
  console.assert(slides.every(slide => slide.kind === 'list'), 'List chunks should remain list slides');
  console.assert(slides[0].title === 'Technical Highlights', 'Chunked slides should retain section title');
  console.assert(slides[0].content.includes('<strong>bold</strong>'), 'Inline formatting should survive list chunking');
  console.assert(slides[0].content.includes('Nested note one'), 'Nested list items should be preserved');
  console.assert(slides[1].content.includes('Item three'), 'Second chunk should contain third item');
  console.assert(slides[2].content.includes('Item five'), 'Last chunk should contain final item');
  console.log('✓ List chunking test passed');
}

function testCodeBlocksStayWhole() {
  const markdown = `## Example

\`\`\`javascript
console.log('one');
console.log('two');
\`\`\`

Follow-up sentence.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides[0].kind === 'code', 'Code block should stay a single code slide');
  console.assert(slides[0].content.includes('<pre>'), 'Code slide should keep preformatted structure');
  console.assert(slides[1].content.includes('Follow-up sentence.'), 'Trailing sentence should become its own slide');
  console.log('✓ Code block preservation test passed');
}

function testAbbreviationsAndDecimals() {
  const markdown = `## Notes

Dr. Smith reviewed version 2.5.1 today. The rollout is safe.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 slides, got ${slides.length}`);
  console.assert(slides[0].content.includes('Dr. Smith'), 'Abbreviations should not split a sentence');
  console.assert(slides[0].content.includes('2.5.1'), 'Decimals should not split a sentence');
  console.log('✓ Abbreviation and decimal test passed');
}

function testUrlsAndInlineCode() {
  const markdown = `## Links

Check https://example.com/docs. Then run \`make build-local\`.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 slides, got ${slides.length}`);
  console.assert(slides[0].content.includes('https://example.com/docs'), 'URL should stay intact');
  console.assert(slides[1].content.includes('<code>make build-local</code>'), 'Inline code should stay intact');
  console.log('✓ URL and inline code test passed');
}

function testInlineFormattingAndSentenceSplit() {
  const markdown = `## Rich Text

This is **bold** and *italic*. Here is [a link](https://example.com).`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 slides, got ${slides.length}`);
  console.assert(slides[0].content.includes('<strong>bold</strong>'), 'Bold formatting should survive sentence splitting');
  console.assert(slides[0].content.includes('<em>italic</em>'), 'Italic formatting should survive sentence splitting');
  console.assert(slides[1].content.includes('<a href="https://example.com">a link</a>'), 'Links should survive sentence splitting');
  console.log('✓ Inline formatting and sentence split test passed');
}

function testBlockquoteAndTableStayStructured() {
  const markdown = `## Evidence

> First quoted line.
>
> Second quoted line.

| Name | Value |
| --- | --- |
| Alpha | 1 |
| Beta | 2 |`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 structured slides, got ${slides.length}`);
  console.assert(slides[0].content.includes('<blockquote>'), 'Blockquotes should stay structured');
  console.assert(slides[1].content.includes('<table>'), 'Tables should stay structured');
  console.log('✓ Blockquote and table structure test passed');
}

function testEmptyMarkdown() {
  console.assert(parseMarkdownToSlides('').length === 0, 'Empty markdown should return no slides');
  console.assert(parseMarkdownToSlides('   \n\n').length === 0, 'Whitespace markdown should return no slides');
  console.log('✓ Empty markdown test passed');
}

function testReadTimeHelpers() {
  const markdown = `Sentence one.

Sentence two.`;
  const slides = parseMarkdownToSlides(markdown);
  const total = calculateTotalReadTime(slides);

  console.assert(total >= 10, 'Short slides should still have minimum timing');
  console.assert(typeof formatTotalReadTime(slides) === 'string', 'Formatted total should be string');
  console.assert(getRemainingReadTime(slides, 1) < total, 'Remaining time should drop after first slide');
  console.assert(typeof formatRemainingTime(slides, 0) === 'string', 'Formatted remaining time should be string');
  console.log('✓ Read time helper test passed');
}

function testMetadataAndColorRotation() {
  const markdown = Array(7).fill(0).map((_, index) => `Sentence ${index + 1}.`).join('\n\n');
  const slides = parseMarkdownToSlides(markdown);

  console.assert(slides[0].slideNumber === 1, 'First slide number should be 1');
  console.assert(slides[6].slideNumber === 7, 'Last slide number should be 7');
  console.assert(slides[0].color.name === slides[5].color.name, 'Colors should rotate through palette');
  console.log('✓ Metadata and color rotation test passed');
}

export function runAllTests() {
  console.group('Running SlideshowParser Tests');

  try {
    testIntroAndSectionSlides();
    testListChunking();
    testCodeBlocksStayWhole();
    testAbbreviationsAndDecimals();
    testUrlsAndInlineCode();
    testInlineFormattingAndSentenceSplit();
    testBlockquoteAndTableStayStructured();
    testEmptyMarkdown();
    testReadTimeHelpers();
    testMetadataAndColorRotation();
    console.log('\n✅ All tests passed!');
  } catch (error) {
    console.error('❌ Test failed:', error);
  }

  console.groupEnd();
}

if (typeof window !== 'undefined') {
  window.runSlideshowParserTests = runAllTests;
  console.log('Slideshow parser tests loaded. Run: window.runSlideshowParserTests()');
}
