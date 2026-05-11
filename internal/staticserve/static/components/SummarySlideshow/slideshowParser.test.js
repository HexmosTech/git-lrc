import {
  calculateTotalReadTime,
  evaluateSummarySlidesEligibility,
  formatRemainingTime,
  formatTotalReadTime,
  getRemainingReadTime,
  parseMarkdownToSlides
} from './slideshowParser.js';
import {
  buildChapterNavigation,
  buildChapterExplorerCards,
  buildProgressTrackItems,
  getActiveProgressTrackItemKey,
  getActiveProgressTrackMarkerKey
} from './SummarySlideshow.js';

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
  console.assert(slides.length === 5, `Expected 5 list slides (one per item), got ${slides.length}`);
  console.assert(slides.every(slide => slide.kind === 'list'), 'Generic list items should remain list slides');
  console.assert(slides[0].title === 'Technical Highlights', 'List slides should retain section title');
  console.assert(slides[0].content.includes('<strong>bold</strong>'), 'Inline formatting should survive list splitting');
  console.assert(slides[1].content.includes('Nested note one'), 'Nested list items should stay with their parent item');
  console.assert(slides[2].content.includes('Item three'), 'Third item should be on its own slide');
  console.assert(slides[4].content.includes('Item five'), 'Last item should be on its own slide');
  console.log('✓ List one-item-per-slide test passed');
}

function testStructuredFilePoints() {
  const markdown = `## Technical Highlights

- internal/staticserve/static/components/Summary.js: Refactored summary view mode control.
- internal/staticserve/static/components/SummarySlideshow/SummarySlideshow.js: Added dark-theme rendering and structured file cards.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 file-point slides, got ${slides.length}`);
  console.assert(slides.every(slide => slide.kind === 'file-point'), 'Structured file bullets should become file-point slides');
  console.assert(slides[0].meta?.pathShort === 'Summary.js', 'First file-point should shorten to file name');
  console.assert(slides[1].meta?.filePath === 'internal/staticserve/static/components/SummarySlideshow/SummarySlideshow.js', 'Second file-point should preserve full file path metadata');
  console.log('✓ Structured file-point test passed');
}

function testBareFilenameBecomesFilePoint() {
  const markdown = `## Technical Highlights

- slideshowParser.js: Adds one-point-per-slide list behavior.
- SummarySlideshow.js: Improves interactive file-path rendering.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 file-point slides for bare filenames, got ${slides.length}`);
  console.assert(slides.every(slide => slide.kind === 'file-point'), 'Bare filename bullets should become file-point slides when uniquely resolvable later');
  console.assert(slides[0].meta?.filePath === 'slideshowParser.js', 'First bare filename should be stored as file metadata');
  console.assert(slides[1].meta?.filePath === 'SummarySlideshow.js', 'Second bare filename should be stored as file metadata');
  console.log('✓ Bare filename file-point test passed');
}

function testFilePointsBecomeNestedChapterMarkers() {
  const markdown = `# Review Summary

## Technical Highlights

- internal/staticserve/static/components/Summary.js: Wires the slideshow entry point.
- internal/staticserve/static/components/SummarySlideshow/SummarySlideshow.js: Adds direct subsection markers on the progress bar.
- internal/staticserve/static/components/SummarySlideshow/SummarySlideshow.js: Keeps repeated file slides grouped together.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);

  console.assert(chapters.length === 1, `Expected a single top-level chapter, got ${chapters.length}`);
  console.assert(chapters[0].title === 'Technical Highlights', 'Top-level chapter should remain Technical Highlights');
  console.assert(chapters[0].subchapters.length === 2, `Expected 2 nested file markers, got ${chapters[0].subchapters.length}`);
  console.assert(chapters[0].subchapters[0].title === 'Summary.js', 'First file point should become a nested marker using the short path');
  console.assert(chapters[0].subchapters[1].title === 'SummarySlideshow.js', 'Second file point should become a nested marker using the short path');
  console.assert(chapters[0].subchapters[1].slideCount === 2, 'Repeated file slides should stay grouped into one nested marker');
  console.log('✓ File-point nested chapter marker test passed');
}

function testStructuredLabelPoints() {
  const markdown = `## Impact

- Functionality: Users can now open specific files from slideshow points.
- Risk: Long paths may reduce readability without structured formatting.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 label-point slides, got ${slides.length}`);
  console.assert(slides.every(slide => slide.kind === 'label-point'), 'Functionality/Risk bullets should become label-point slides');
  console.assert(slides[0].meta?.label.toLowerCase() === 'functionality', 'First label-point should preserve label');
  console.assert(slides[1].content.includes('Long paths'), 'Label-point should preserve body text');
  console.log('✓ Structured label-point test passed');
}

function testStructuredLabelPointsBecomeNestedChapterMarkers() {
  const markdown = `# Review Summary

## Impact

- Functionality: Users can jump to the functional outcome directly.
- Risk: Risk slides remain reachable from the chapter popover.
- Risk: Multiple risk slides should stay grouped under the same nested marker.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);

  console.assert(chapters.length === 1, `Expected a single top-level chapter, got ${chapters.length}`);
  console.assert(chapters[0].title === 'Impact', 'Top-level chapter should remain Impact');
  console.assert(chapters[0].subchapters.length === 2, `Expected 2 nested markers from structured labels, got ${chapters[0].subchapters.length}`);
  console.assert(chapters[0].subchapters[0].title === 'Functionality', 'Functionality should become a nested chapter marker');
  console.assert(chapters[0].subchapters[1].title === 'Risk', 'Risk should become a nested chapter marker');
  console.assert(chapters[0].subchapters[1].slideCount === 2, 'Repeated Risk slides should stay grouped into one nested marker');
  console.log('✓ Structured label nested chapter marker test passed');
}

function testMixedListStaysSinglePointPerSlide() {
  const markdown = `## Mixed List

- Functionality: Open files directly from slides.
- Item without structure.
- internal/staticserve/static/styles.css: Refines point-slide styling.
- Another plain item.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 4, `Expected 4 slides for 4 list points, got ${slides.length}`);
  console.assert(slides[0].kind === 'label-point', 'First point should be a label-point slide');
  console.assert(slides[1].kind === 'list', 'Second point should remain a generic list slide');
  console.assert(slides[2].kind === 'file-point', 'Third point should be a file-point slide');
  console.assert(slides[3].kind === 'list', 'Fourth point should remain a generic list slide');
  console.log('✓ Mixed list one-item-per-slide test passed');
}

function testSlidesCaptureChapterHierarchy() {
  const markdown = `# Review Summary

## Impact

### Functionality

Users can jump straight to the relevant chapter.

### Risk

Incorrect metadata would produce broken chapter boundaries.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides[1].chapter?.topLevelTitle === 'Impact', 'Top-level chapter title should be preserved on nested slides');
  console.assert(slides[1].chapter?.nestedTitle === 'Functionality', 'Nested chapter title should be preserved');
  console.assert(slides[2].chapter?.topLevelTitle === 'Impact', 'Later nested slides should keep the same top-level title');
  console.assert(slides[2].chapter?.nestedTitle === 'Risk', 'Later nested slides should update the nested title');
  console.assert(slides[1].chapter?.pathKey === 'impact::functionality', 'Nested chapter path key should be deterministic');
  console.log('✓ Slide chapter hierarchy metadata test passed');
}

function testChapterNavigationModelGroupsIntroAndNestedSections() {
  const markdown = `# Review Summary

## Overview

Lead sentence.

## Impact

### Functionality

Functional outcome.

### Risk

Risk outcome.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);

  console.assert(chapters.length === 2, `Expected 2 top-level chapters, got ${chapters.length}`);
  console.assert(chapters[0].title === 'Overview', 'Intro slide should fold into the first explicit top-level chapter');
  console.assert(chapters[0].startIndex === 0, 'First chapter should start at the intro slide');
  console.assert(chapters[1].title === 'Impact', 'Second top-level chapter should be Impact');
  console.assert(chapters[1].subchapters.length === 2, `Expected 2 nested chapter markers, got ${chapters[1].subchapters.length}`);
  console.assert(chapters[1].subchapters[0].title === 'Functionality', 'First nested chapter should be Functionality');
  console.assert(chapters[1].subchapters[1].title === 'Risk', 'Second nested chapter should be Risk');
  console.log('✓ Chapter navigation grouping test passed');
}

function testChaptersWithoutNamedSubsectionsGetSlideMarkers() {
  const markdown = `# Review Summary

## Overview

First overview sentence.

Second overview sentence.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);

  console.assert(chapters.length === 1, `Expected 1 top-level chapter, got ${chapters.length}`);
  console.assert(chapters[0].subchapters.length === 3, `Expected 3 markers including the intro-folded slide, got ${chapters[0].subchapters.length}`);
  console.assert(chapters[0].subchapters[0].title === 'Overview 1', 'First synthetic marker should be Overview 1');
  console.assert(chapters[0].subchapters[1].title === 'Overview 2', 'Second synthetic marker should be Overview 2');
  console.assert(chapters[0].subchapters[2].title === 'Overview 3', 'Third synthetic marker should be Overview 3');
  console.assert(chapters[0].subchapters.every(subchapter => subchapter.isSynthetic === true), 'Overview-only markers should all be synthetic');
  console.log('✓ Synthetic slide marker test passed');
}

function testProgressTrackIncludesCompleteItem() {
  const markdown = `# Review Summary

## Overview

Lead sentence.

## Impact

Risk outcome.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);
  const trackItems = buildProgressTrackItems(chapters, slides.length);
  const completeTrackItem = trackItems[trackItems.length - 1];
  const totalUnitCount = trackItems.reduce((sum, trackItem) => sum + trackItem.unitCount, 0);

  console.assert(trackItems.length === 3, `Expected 3 track items including Complete, got ${trackItems.length}`);
  console.assert(completeTrackItem.key === 'complete', 'Final track item should use the complete key');
  console.assert(completeTrackItem.kind === 'complete', 'Final track item should be marked as complete');
  console.assert(completeTrackItem.startIndex === slides.length, 'Complete track item should point at the final completion slide index');
  console.assert(completeTrackItem.subchapters.length === 1, 'Complete track item should expose a single marker');
  console.assert(completeTrackItem.subchapters[0].key === 'complete::marker', 'Complete marker should have a stable dedicated key');
  console.assert(completeTrackItem.subchapters[0].offsetPct === 0, 'Complete marker should align to the start of its chunk like other markers');
  console.assert(totalUnitCount === slides.length + 1, `Expected track units to equal slides plus completion, got ${totalUnitCount}`);
  console.log('✓ Progress track includes Complete item test passed');
}

function testCompleteScreenActivatesCompleteTrackItem() {
  const markdown = `# Review Summary

## Overview

Lead sentence.

## Impact

Risk outcome.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);
  const trackItems = buildProgressTrackItems(chapters, slides.length);

  console.assert(getActiveProgressTrackItemKey(trackItems, slides.length) === 'complete', 'Complete screen should activate the Complete track item');
  console.assert(getActiveProgressTrackMarkerKey(trackItems, slides.length) === 'complete::marker', 'Complete screen should activate the Complete marker');
  console.assert(getActiveProgressTrackItemKey(trackItems, slides.length - 1) !== 'complete', 'Last real slide should not activate the Complete track item');
  console.log('✓ Complete screen track activation test passed');
}

function testChapterExplorerCardsIncludeAllSections() {
  const markdown = `# Review Summary

## Overview

Lead sentence.

## Impact

### Functionality

Functional outcome.

### Risk

Risk outcome.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);
  const trackItems = buildProgressTrackItems(chapters, slides.length);
  const cards = buildChapterExplorerCards(trackItems, 2, 'impact', 'impact::functionality');

  console.assert(cards.length === 3, `Expected 3 explorer cards including Complete, got ${cards.length}`);
  console.assert(cards[0].title === 'Overview', 'First explorer card should be Overview');
  console.assert(cards[1].title === 'Impact', 'Second explorer card should be Impact');
  console.assert(cards[2].title === 'Complete', 'Final explorer card should be Complete');
  console.assert(cards[1].isActive === true, 'Active progress item should map to the active explorer card');
  console.assert(cards[1].subchapters.length === 2, 'Impact explorer card should expose nested subchapter actions');
  console.assert(cards[1].subchapters[0].isActive === true, 'Active progress marker should map to the active explorer subchapter');
  console.assert(cards[2].subchapters.length === 0, 'Complete explorer card should not expose redundant nested subchapter actions');
  console.log('✓ Chapter explorer card list test passed');
}

function testChapterExplorerCardsTrackProgressFill() {
  const markdown = `# Review Summary

## Overview

First overview sentence.

Second overview sentence.`;

  const slides = parseMarkdownToSlides(markdown);
  const chapters = buildChapterNavigation(slides);
  const trackItems = buildProgressTrackItems(chapters, slides.length);
  const cards = buildChapterExplorerCards(trackItems, 1, 'overview', 'overview::slide-2');

  console.assert(cards[0].progressPercent > 0, 'Current explorer card should carry a positive progress fill');
  console.assert(cards[0].progressPercent < 100, 'Explorer card progress should reflect in-progress chapters');
  console.assert(cards[1].progressPercent === 0, 'Complete explorer card should remain unfilled before the completion slide');
  console.log('✓ Chapter explorer card progress test passed');
}

function testSingleListSlideDoesNotKeepWrapperBullet() {
  const markdown = `## Technical Highlights

- One bullet point only.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 1, `Expected one slide, got ${slides.length}`);
  console.assert(slides[0].kind === 'list', 'Single bullet should still be treated as list content');
  console.assert(!slides[0].content.includes('<ul'), 'Single-point slide should not preserve the UL wrapper');
  console.assert(!slides[0].content.includes('<li'), 'Single-point slide should not preserve the LI wrapper');
  console.log('✓ Single list point de-bullet test passed');
}

function testRiskLabelUsesRiskPalette() {
  const markdown = `## Impact

- Risk: Deployment can fail if stale hooks are still installed.
- Risk: Mismatched path aliases can hide relevant review points.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 2, `Expected 2 risk label-point slides, got ${slides.length}`);
  console.assert(slides.every(slide => slide.kind === 'label-point'), 'Risk entries should be label-point slides');
  console.assert(slides[0].color.name.startsWith('risk-'), 'First risk slide should use risk color palette');
  console.assert(slides[1].color.name.startsWith('risk-'), 'Second risk slide should use risk color palette');
  console.assert(slides[0].color.name !== slides[1].color.name, 'Risk slides should rotate within risk palette');
  console.log('✓ Risk semantic palette test passed');
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

function testStructuredPointsPreserveInlineCode() {
  const markdown = `## Technical Highlights

- **internal/staticserve/static/components/review_outcome_state.mjs**: The \`shouldShowAllClear\` utility function now accepts and uses \`summarySlidesEligibility\`.
- **Functionality**: The \`SummarySlideshow\` renderer now preserves \`code\` formatting inside structured points.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides[0].kind === 'file-point', 'Structured file point should remain a file-point slide');
  console.assert(slides[0].content.includes('<code>shouldShowAllClear</code>'), 'File-point description should preserve inline code');
  console.assert(slides[0].content.includes('<code>summarySlidesEligibility</code>'), 'File-point description should preserve all inline code tokens');
  console.assert(slides[1].kind === 'label-point', 'Structured label point should remain a label-point slide');
  console.assert(slides[1].content.includes('<code>SummarySlideshow</code>'), 'Label-point body should preserve inline code');
  console.assert(slides[1].content.includes('<code>code</code>'), 'Label-point body should preserve repeated inline code');
  console.log('✓ Structured point inline code preservation test passed');
}

function testStructuredFilePointsDoNotLeaveEmptyInlineArtifacts() {
  const markdown = `## Technical Highlights

- \`SummarySlideshow.js\`: Introduces explicit Previous/Next slide navigation buttons with keyboard support.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 1, `Expected 1 file-point slide, got ${slides.length}`);
  console.assert(slides[0].kind === 'file-point', 'Backticked file prefix should still parse as a file-point slide');
  console.assert(!slides[0].content.includes('<code></code>'), 'Structured file-point body should not keep empty code wrappers after prefix removal');
  console.assert(!/^<code>\s*<\/code>/.test(slides[0].content), 'Structured file-point body should not start with an empty code artifact');
  console.assert(slides[0].content.includes('Introduces explicit Previous/Next slide navigation buttons with keyboard support.'), 'Structured file-point body should retain the description after prefix removal');
  console.log('✓ Structured file-point empty inline artifact test passed');
}

function testFullSummaryKeepsRiskSlidesAndAvoidsDuplication() {
  const markdown = `# Review Summary

## Overview

The UI state logic now depends on this new eligibility status.

## Technical Highlights

- **internal/staticserve/static/app.js**: Passes \`summarySlidesEligibility\` status to the \`shouldShowAllClear\` helper function.
- **internal/staticserve/static/components/review_outcome_state.mjs**: Uses the existing validator result instead of duplicating section checks.

## Impact

- **Impact**: The 'all clear' UI state now accurately reflects the readiness and quality of structured summaries.

## Risks

- **Risk**: Reviews with malformed structured summaries should not show the success banner.
- **Risk**: Inline code formatting must remain visible without shrinking the surrounding slide body text.`;

  const slides = parseMarkdownToSlides(markdown);
  console.assert(slides.length === 7, `Expected 7 slides before appreciation, got ${slides.length}`);
  console.assert(slides[0].kind === 'intro', 'First slide should remain the intro slide');
  console.assert(slides[1].kind === 'sentence', 'Overview should remain a sentence slide');
  console.assert(slides[2].kind === 'file-point', 'First technical highlight should be a file-point slide');
  console.assert(slides[3].kind === 'file-point', 'Second technical highlight should be a file-point slide');
  console.assert(slides[4].kind === 'label-point', 'Impact should remain a dedicated label-point slide');
  console.assert(slides[5].kind === 'label-point', 'First risk should remain a dedicated label-point slide');
  console.assert(slides[6].kind === 'label-point', 'Second risk should remain a dedicated label-point slide');
  console.assert(slides[5].meta?.label.toLowerCase() === 'risk', 'First risk label should be preserved');
  console.assert(slides[6].meta?.label.toLowerCase() === 'risk', 'Second risk label should be preserved');
  console.assert(slides[4].content.match(/readiness and quality of structured summaries/g)?.length === 1, 'Impact body should not duplicate sentence content');
  console.assert(slides[2].content.includes('<code>summarySlidesEligibility</code>'), 'Technical highlight should preserve inline code');
  console.assert(slides[2].content.match(/summarySlidesEligibility/g)?.length === 1, 'Structured file-point body should not duplicate inline code text');
  console.log('✓ Full summary ordering and duplication regression test passed');
}

function testBoldStructuredPrefixesSplitSentencesWithoutCarryover() {
  const markdown = `# Review Summary

## Overview

The review summary should preserve structure and sentence boundaries.

## Technical Highlights

- **internal/staticserve/static/components/review_performance_state.mjs**: Refined the \`First comment\` metric for zero-comment reviews. It now renders \`No comments\` after completion.

## Impact

- **Functionality**: Successful zero-comment reviews no longer say Waiting. Reviewers can distinguish in-progress work from completed zero-comment reviews. The metric language now matches the actual outcome.

## Risks

- **Risk**: Bolded structured prefixes must still parse correctly. Inline \`code\` must stay formatted.`;

  const slides = parseMarkdownToSlides(markdown);

  console.assert(slides.length === 9, `Expected 9 slides before appreciation, got ${slides.length}`);
  console.assert(slides[2].kind === 'file-point', 'First technical sentence should be a file-point slide');
  console.assert(slides[3].kind === 'file-point', 'Second technical sentence should be a file-point slide');
  console.assert(slides[2].content.includes('<code>First comment</code>'), 'Technical highlight should preserve inline code in the first sentence');
  console.assert(slides[3].content.includes('<code>No comments</code>'), 'Technical highlight should preserve inline code in the second sentence');
  console.assert(slides[4].kind === 'label-point', 'First impact sentence should be its own label-point slide');
  console.assert(slides[5].kind === 'label-point', 'Second impact sentence should be its own label-point slide');
  console.assert(slides[6].kind === 'label-point', 'Third impact sentence should be its own label-point slide');
  console.assert(slides[4].content.includes('no longer say Waiting'), 'First impact slide should keep only the first sentence');
  console.assert(slides[5].content.includes('distinguish in-progress work'), 'Second impact slide should keep only the second sentence');
  console.assert(slides[6].content.includes('metric language now matches'), 'Third impact slide should keep only the third sentence');
  console.assert(!slides[4].content.includes('First comment'), 'Impact slides should not inherit technical highlight content');
  console.assert(slides[7].kind === 'label-point', 'First risk sentence should be its own label-point slide');
  console.assert(slides[8].kind === 'label-point', 'Second risk sentence should be its own label-point slide');
  console.assert(slides[8].content.includes('<code>code</code>'), 'Risk slides should preserve inline code after sentence splitting');
  console.assert(!slides[7].content.includes('metric language now matches'), 'Risk slides should not inherit impact content');
  console.log('✓ Bold structured-prefix sentence split regression test passed');
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

function testSlidesEligibilityRequiresSections() {
  const valid = `# Review Summary

## Overview

This change improves line navigation and summary rendering behavior.

## Technical Highlights

- internal/staticserve/static/app.js: Adds resilient slideshow gating.

## Impact

- Functionality: Slides now appear only for valid summary structures.`;

  const invalidMissingImpact = `# Review Summary

## Overview

This is a proper overview section.

## Technical Highlights

- internal/staticserve/static/app.js: Adds resilient slideshow gating.`;

  const validResult = evaluateSummarySlidesEligibility(valid);
  console.assert(validResult.eligible === true, 'Required sections with content should be eligible for slides');

  const missingResult = evaluateSummarySlidesEligibility(invalidMissingImpact);
  console.assert(missingResult.eligible === false, 'Missing required section should be ineligible for slides');
  console.assert(missingResult.reason === 'missing-required-sections', 'Missing section reason should be reported');
  console.log('✓ Slides eligibility required sections test passed');
}

function testSlidesEligibilityRejectsEmptySections() {
  const markdown = `# Review Summary

## Overview

Valid overview body text is present here.

## Technical Highlights

Done.

## Impact

Okay.`;

  const result = evaluateSummarySlidesEligibility(markdown);
  console.assert(result.eligible === false, 'Sections with tiny bodies should be ineligible');
  console.assert(result.reason === 'empty-required-sections', 'Empty section reason should be reported');
  console.log('✓ Slides eligibility empty section test passed');
}

function testSlidesEligibilityAllowsAliases() {
  const markdown = `# Review Summary

## Summary

This section introduces the review output and key context in a concise way.

## Highlights

- internal/staticserve/static/app.js: Adds centralized slideshow eligibility gating.
- internal/staticserve/static/components/Summary.js: Defaults to text mode when slides are disabled.

## Risks

- Functionality: Aliased headings still unlock slides with predictable structure.`;

  const result = evaluateSummarySlidesEligibility(markdown);
  console.assert(result.eligible === true, 'Known heading aliases should be eligible for slides');
  console.assert(result.reason === 'ok', 'Known heading aliases should pass with reason ok');
  console.log('✓ Slides eligibility alias headings test passed');
}

export function runAllTests() {
  console.group('Running SlideshowParser Tests');

  try {
    testIntroAndSectionSlides();
    testListChunking();
    testStructuredFilePoints();
    testBareFilenameBecomesFilePoint();
    testFilePointsBecomeNestedChapterMarkers();
    testStructuredLabelPoints();
    testStructuredLabelPointsBecomeNestedChapterMarkers();
    testMixedListStaysSinglePointPerSlide();
    testSlidesCaptureChapterHierarchy();
    testChapterNavigationModelGroupsIntroAndNestedSections();
    testChaptersWithoutNamedSubsectionsGetSlideMarkers();
    testProgressTrackIncludesCompleteItem();
    testCompleteScreenActivatesCompleteTrackItem();
    testChapterExplorerCardsIncludeAllSections();
    testChapterExplorerCardsTrackProgressFill();
    testSingleListSlideDoesNotKeepWrapperBullet();
    testRiskLabelUsesRiskPalette();
    testCodeBlocksStayWhole();
    testAbbreviationsAndDecimals();
    testUrlsAndInlineCode();
    testStructuredPointsPreserveInlineCode();
    testStructuredFilePointsDoNotLeaveEmptyInlineArtifacts();
    testFullSummaryKeepsRiskSlidesAndAvoidsDuplication();
    testBoldStructuredPrefixesSplitSentencesWithoutCarryover();
    testInlineFormattingAndSentenceSplit();
    testBlockquoteAndTableStayStructured();
    testEmptyMarkdown();
    testReadTimeHelpers();
    testMetadataAndColorRotation();
    testSlidesEligibilityRequiresSections();
    testSlidesEligibilityRejectsEmptySections();
    testSlidesEligibilityAllowsAliases();
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
