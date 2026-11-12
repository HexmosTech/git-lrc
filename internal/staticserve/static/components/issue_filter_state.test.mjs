import test from 'node:test';
import assert from 'node:assert/strict';

import {
    buildCommentVisibilityKey,
    buildIssueCategoryGroups,
    buildIssueFacetOptions,
    buildIssueFilterUniverse,
    countFileVisibleIssues,
    countIssuesByFilters,
    createDefaultIssueFilters,
    getCommentFilterValue,
    getIssueFilterStats,
    getIssueFilterSummary,
    hasActiveIssueFilters,
    matchesIssueFilters,
    normalizeIssueFilters,
    resetIssueFilters,
    toggleIssueFilterValue,
} from './issue_filter_state.mjs';

function buildFiles() {
    return [
        {
            FilePath: 'README.md',
            Hunks: [
                {
                    Lines: [
                        {
                            IsComment: true,
                            Comments: [
                                {
                                    Severity: 'CRITICAL',
                                    Confidence: 'High',
                                    Type: 'Best Practice',
                                    Category: 'Documentation',
                                    Subcategory: 'Missing Prerequisites',
                                    Content: 'Document the runtime requirements.',
                                    Line: 12,
                                },
                                {
                                    Severity: 'WARNING',
                                    Confidence: 'Medium',
                                    Type: 'Risk',
                                    Category: 'Documentation',
                                    Subcategory: 'Broken Links',
                                    Content: 'One of the links is stale.',
                                    Line: 20,
                                },
                            ],
                        },
                    ],
                },
            ],
        },
        {
            FilePath: 'parser.go',
            Hunks: [
                {
                    Lines: [
                        {
                            IsComment: true,
                            Comments: [
                                {
                                    Severity: 'ERROR',
                                    Confidence: 'High',
                                    Type: 'Bug',
                                    Category: 'Logic',
                                    Subcategory: 'Parser Mismatch',
                                    Content: 'The parser contract is inconsistent.',
                                    Line: 7,
                                },
                                {
                                    Severity: 'INFO',
                                    Confidence: 'Low',
                                    Type: 'Optimization',
                                    Category: 'Style',
                                    Subcategory: 'String Processing',
                                    Content: 'Combine string transforms.',
                                    Line: 13,
                                },
                            ],
                        },
                    ],
                },
            ],
        },
    ];
}

test('createDefaultIssueFilters starts with no disabled facet values', () => {
    const filters = createDefaultIssueFilters();

    assert.deepEqual([...filters.disabled.severities], []);
    assert.deepEqual([...filters.disabled.confidences], []);
    assert.deepEqual([...filters.disabled.types], []);
    assert.deepEqual([...filters.disabled.categories], []);
    assert.deepEqual([...filters.disabled.subcategories], []);
    assert.equal(hasActiveIssueFilters(filters), false);
});

test('matchesIssueFilters applies multi-factor enabled semantics', () => {
    const filters = normalizeIssueFilters({
        disabled: {
            severities: new Set(['error', 'info']),
            confidences: new Set(['medium', 'low']),
            categories: new Set(['logic', 'style']),
        },
    });

    assert.equal(matchesIssueFilters({ Severity: 'CRITICAL', Confidence: 'High', Category: 'Documentation' }, filters), true);
    assert.equal(matchesIssueFilters({ Severity: 'WARNING', Confidence: 'Medium', Category: 'Documentation' }, filters), false);
    assert.equal(matchesIssueFilters({ Severity: 'ERROR', Confidence: 'High', Category: 'Logic' }, filters), false);
});

test('buildIssueFacetOptions narrows subcategories by the enabled main-category filter', () => {
    const filters = normalizeIssueFilters({
        disabled: {
            categories: new Set(['logic', 'style']),
        },
    });

    const options = buildIssueFacetOptions(buildFiles(), filters, new Set());

    assert.deepEqual(
        options.subcategories.map((option) => [option.value, option.count]),
        [
            ['broken links', 1],
            ['missing prerequisites', 1],
        ],
    );
    assert.deepEqual(
        options.severities.map((option) => [option.value, option.count]),
        [
            ['critical', 1],
            ['warning', 1],
            ['info', 0],
        ],
    );
});

test('buildIssueFacetOptions treats confidence and type filters as active by default', () => {
    const options = buildIssueFacetOptions(buildFiles(), createDefaultIssueFilters(), new Set());

    assert.deepEqual(
        options.confidences.map((option) => [option.value, option.active]),
        [
            ['high', true],
            ['medium', true],
            ['low', true],
        ],
    );
    assert.deepEqual(
        options.types.map((option) => [option.value, option.active]),
        [
            ['best practice', true],
            ['bug', true],
            ['optimization', true],
            ['risk', true],
        ],
    );
});

test('buildIssueCategoryGroups preserves the category to subcategory relationship visually', () => {
    const groups = buildIssueCategoryGroups(buildFiles(), normalizeIssueFilters({}), new Set());

    assert.deepEqual(
        groups.map((group) => ({
            label: group.label,
            subcategories: group.subcategories.map((subcategory) => subcategory.label),
        })),
        [
            {
                label: 'Documentation',
                subcategories: ['Broken Links', 'Missing Prerequisites'],
            },
            {
                label: 'Logic',
                subcategories: ['Parser Mismatch'],
            },
            {
                label: 'Style',
                subcategories: ['String Processing'],
            },
        ],
    );
});

test('buildIssueCategoryGroups keeps category order stable when a category becomes the only enabled one', () => {
    const files = buildFiles();
    const baseline = buildIssueCategoryGroups(files, normalizeIssueFilters({}), new Set());
    const active = buildIssueCategoryGroups(files, normalizeIssueFilters({
        disabled: {
            categories: new Set(['documentation', 'style']),
        },
    }), new Set());

    assert.deepEqual(
        baseline.map((group) => group.label),
        active.map((group) => group.label),
    );
});

test('buildIssueCategoryGroups treats all categories and subcategories as active by default', () => {
    const groups = buildIssueCategoryGroups(buildFiles(), normalizeIssueFilters({}), new Set());

    assert.deepEqual(
        groups.map((group) => ({
            label: group.label,
            active: group.active,
            subcategories: group.subcategories.map((subcategory) => ({
                label: subcategory.label,
                active: subcategory.active,
            })),
        })),
        [
            {
                label: 'Documentation',
                active: true,
                subcategories: [
                    { label: 'Broken Links', active: true },
                    { label: 'Missing Prerequisites', active: true },
                ],
            },
            {
                label: 'Logic',
                active: true,
                subcategories: [{ label: 'Parser Mismatch', active: true }],
            },
            {
                label: 'Style',
                active: true,
                subcategories: [{ label: 'String Processing', active: true }],
            },
        ],
    );
});

test('toggleIssueFilterValue disables a main category together with its subcategories', () => {
    const next = toggleIssueFilterValue(createDefaultIssueFilters(), 'category', 'documentation', {
        childValues: ['broken links', 'missing prerequisites'],
    });

    assert.deepEqual([...next.disabled.categories].sort(), ['documentation']);
    assert.deepEqual([...next.disabled.subcategories].sort(), ['broken links', 'missing prerequisites']);
});

test('toggleIssueFilterValue re-enables a disabled main category together with its subcategories', () => {
    const disabled = toggleIssueFilterValue(createDefaultIssueFilters(), 'category', 'documentation', {
        childValues: ['broken links', 'missing prerequisites'],
    });
    const next = toggleIssueFilterValue(disabled, 'category', 'documentation', {
        childValues: ['broken links', 'missing prerequisites'],
    });

    assert.deepEqual([...next.disabled.categories], []);
    assert.deepEqual([...next.disabled.subcategories], []);
});

test('toggleIssueFilterValue toggles an individual subcategory off and back on', () => {
    const disabled = toggleIssueFilterValue(createDefaultIssueFilters(), 'subcategory', 'broken links');

    assert.deepEqual([...disabled.disabled.subcategories].sort(), ['broken links']);

    const reenabled = toggleIssueFilterValue(disabled, 'subcategory', 'broken links');

    assert.deepEqual([...reenabled.disabled.subcategories], []);
});

test('toggleIssueFilterValue can disable every confidence value without re-enabling all on the last click', () => {
    let next = createDefaultIssueFilters();
    next = toggleIssueFilterValue(next, 'confidence', 'low');
    next = toggleIssueFilterValue(next, 'confidence', 'medium');
    next = toggleIssueFilterValue(next, 'confidence', 'high');

    assert.deepEqual([...next.disabled.confidences].sort(), ['high', 'low', 'medium']);
    assert.equal(hasActiveIssueFilters(next), true);
    assert.equal(matchesIssueFilters({ Severity: 'CRITICAL', Confidence: 'High', Category: 'Documentation' }, next), false);
});

test('countIssuesByFilters and countFileVisibleIssues exclude hidden comments from visible totals', () => {
    const files = buildFiles();
    const filters = normalizeIssueFilters({
        disabled: {
            categories: new Set(['logic', 'style']),
        },
    });
    const hiddenCommentKeys = new Set([
        buildCommentVisibilityKey('README.md', {
            Severity: 'WARNING',
            Confidence: 'Medium',
            Type: 'Risk',
            Category: 'Documentation',
            Subcategory: 'Broken Links',
            Content: 'One of the links is stale.',
            Line: 20,
        }),
    ]);

    const counts = countIssuesByFilters(files, filters, hiddenCommentKeys);

    assert.equal(counts.total, 4);
    assert.equal(counts.visible, 1);
    assert.equal(countFileVisibleIssues(files[0], filters, hiddenCommentKeys), 1);
});

test('getIssueFilterStats returns visible counts and dependent subcategory availability', () => {
    const files = buildFiles();
    const filters = normalizeIssueFilters({
        disabled: {
            categories: new Set(['logic', 'style']),
        },
    });

    const stats = getIssueFilterStats(files, filters, new Set([buildCommentVisibilityKey('parser.go', {
        Severity: 'INFO',
        Confidence: 'Low',
        Type: 'Optimization',
        Category: 'Style',
        Subcategory: 'String Processing',
        Content: 'Combine string transforms.',
        Line: 13,
    })]), (filePath, comment) => buildCommentVisibilityKey(filePath, comment));

    assert.equal(stats.total, 4);
    assert.equal(stats.visible, 2);
    assert.equal(stats.facetCounts.category.get('Documentation'), 2);
    assert.deepEqual([...stats.availableSubcategories].sort(), ['Broken Links', 'Missing Prerequisites']);
});

test('issue filter summary only reports active restrictions beyond defaults', () => {
    const universe = buildIssueFilterUniverse(buildFiles(), new Set());
    const summary = getIssueFilterSummary(normalizeIssueFilters({
        disabled: {
            severities: new Set(['error', 'info']),
            confidences: new Set(['medium', 'low']),
        },
    }), universe);

    assert.deepEqual(summary, ['Severity: 2', 'Confidence: 1']);
});

test('getCommentFilterValue exposes raw main and subcategory facets', () => {
    const [file] = buildFiles();
    const comment = file.Hunks[0].Lines[0].Comments[0];

    assert.equal(getCommentFilterValue(comment, 'category'), 'Documentation');
    assert.equal(getCommentFilterValue(comment, 'subcategory'), 'Missing Prerequisites');
});

test('buildCommentVisibilityKey distinguishes comments across metadata dimensions', () => {
    const left = buildCommentVisibilityKey('README.md', {
        Severity: 'CRITICAL',
        Confidence: 'High',
        Type: 'Best Practice',
        Category: 'Documentation',
        Subcategory: 'Missing Prerequisites',
        Content: 'Document the runtime requirements.',
        Line: 12,
    });
    const right = buildCommentVisibilityKey('README.md', {
        Severity: 'CRITICAL',
        Confidence: 'High',
        Type: 'Risk',
        Category: 'Documentation',
        Subcategory: 'Missing Prerequisites',
        Content: 'Document the runtime requirements.',
        Line: 12,
    });

    assert.notEqual(left, right);
});

test('resetIssueFilters restores defaults', () => {
    const reset = resetIssueFilters();
    assert.deepEqual([...reset.disabled.severities], []);
    assert.deepEqual([...reset.disabled.confidences], []);
    assert.deepEqual([...reset.disabled.tools], []);
});

test('buildIssueFacetOptions includes tool options when tool comments exist', () => {
    const files = [
        {
            FilePath: 'main.go',
            Hunks: [
                {
                    Lines: [
                        {
                            IsComment: true,
                            Comments: [
                                {
                                    Severity: 'critical',
                                    Confidence: 'High',
                                    Type: 'Bug',
                                    Category: 'Security',
                                    Subcategory: 'Secrets Management',
                                    Content: 'Hardcoded secret',
                                    Line: 10,
                                    ToolName: 'gitleaks',
                                    Source: 'tool',
                                },
                                {
                                    Severity: 'warning',
                                    Confidence: 'High',
                                    Type: 'Code Smell',
                                    Category: 'Maintainability',
                                    Subcategory: 'Code Duplication',
                                    Content: 'Duplicate logic',
                                    Line: 20,
                                },
                            ],
                        },
                    ],
                },
            ],
        },
    ];

    const facets = buildIssueFacetOptions(files, createDefaultIssueFilters(), new Set());
    assert.equal(facets.tools.length, 2);
    assert.equal(facets.tools[0].value, 'ai');
    assert.equal(facets.tools[0].label, 'AI Review');
    assert.equal(facets.tools[0].count, 1);
    assert.equal(facets.tools[0].active, true);

    assert.equal(facets.tools[1].value, 'gitleaks');
    assert.equal(facets.tools[1].label, 'gitleaks');
    assert.equal(facets.tools[1].count, 1);
    assert.equal(facets.tools[1].active, true);
});

test('toggleIssueFilterValue filters comments by tool', () => {
    const aiComment = {
        Severity: 'warning',
        Category: 'Style',
        Content: 'Nit',
        Line: 1,
    };
    const toolComment = {
        Severity: 'critical',
        Category: 'Security',
        Content: 'Leaked Key',
        Line: 2,
        ToolName: 'gitleaks',
        Source: 'tool',
    };

    let filters = createDefaultIssueFilters();
    assert.equal(matchesIssueFilters(aiComment, filters), true);
    assert.equal(matchesIssueFilters(toolComment, filters), true);

    filters = toggleIssueFilterValue(filters, 'tool', 'gitleaks');
    assert.equal(matchesIssueFilters(aiComment, filters), true);
    assert.equal(matchesIssueFilters(toolComment, filters), false);

    filters = toggleIssueFilterValue(filters, 'tool', 'gitleaks');
    assert.equal(matchesIssueFilters(aiComment, filters), true);
    assert.equal(matchesIssueFilters(toolComment, filters), true);
});
