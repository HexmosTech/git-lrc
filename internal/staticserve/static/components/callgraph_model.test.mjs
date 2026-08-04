import test from 'node:test';
import assert from 'node:assert/strict';

import {
    buildHierarchy,
    callerGroupLabel,
    emptyCallGraphMessage,
    groupCallers,
    shortName,
} from './callgraph_model.mjs';

test('shortName takes the last dotted segment', () => {
    assert.equal(shortName('a.b.c.RunHelp'), 'RunHelp');
    assert.equal(shortName('RunHelp'), 'RunHelp');
    assert.equal(shortName(''), '');
});

test('buildHierarchy nests callers by their Path', () => {
    const root = buildHierarchy({
        Name: 'leftWidth',
        QualifiedName: 'pkg.leftWidth',
        Callers: [
            { QualifiedName: 'pkg.renderFileList', Depth: 1, Weight: 1 },
            { QualifiedName: 'pkg.View', Depth: 2, Weight: 0.5, Path: ['pkg.renderFileList'] },
        ],
    });

    assert.equal(root.name, 'leftWidth');
    assert.equal(root.children.length, 1, 'the depth-2 caller reuses its via node, not a second child');
    const via = root.children[0];
    assert.equal(via.qualifiedName, 'pkg.renderFileList');
    assert.equal(via.children.length, 1);
    assert.equal(via.children[0].qualifiedName, 'pkg.View');
    assert.equal(via.children[0].depth, 2);
});

test('buildHierarchy ignores PreRename - it must only ever see real graph callers', () => {
    // BlastRadiusPanel's chartSymbol strips PreRename entries before a symbol
    // ever reaches buildHierarchy, so the flag carries no special meaning
    // here - a caller is a caller. This pins that buildHierarchy itself does
    // no rename-aware branching (that logic lives in groupCallers, for the
    // left column only).
    const root = buildHierarchy({
        QualifiedName: 'pkg.leftWidth',
        Callers: [{ QualifiedName: 'pkg.renderPreview', Depth: 1, Weight: 1 }],
    });
    assert.equal(root.children[0].preRename, undefined);
});

test('groupCallers puts pre-rename callers in their own leading group', () => {
    const groups = groupCallers([
        { QualifiedName: 'pkg.liveA', Depth: 1 },
        { QualifiedName: 'pkg.broken', Depth: 1, PreRename: true },
        { QualifiedName: 'pkg.liveB', Depth: 2 },
    ]);

    assert.equal(groups.length, 3);
    assert.equal(groups[0].key, 'pre-rename', 'breaking callers lead - they are the actionable ones');
    assert.deepEqual(groups[0].callers.map((c) => c.QualifiedName), ['pkg.broken']);
    assert.equal(groups[1].depth, 1);
    assert.deepEqual(groups[1].callers.map((c) => c.QualifiedName), ['pkg.liveA']);
    assert.equal(groups[2].depth, 2);
});

test('groupCallers keeps depth ordering when nothing is pre-rename', () => {
    const groups = groupCallers([
        { QualifiedName: 'pkg.c', Depth: 3 },
        { QualifiedName: 'pkg.a', Depth: 1 },
        { QualifiedName: 'pkg.b', Depth: 2 },
    ]);
    assert.deepEqual(groups.map((g) => g.depth), [1, 2, 3]);
    assert.ok(groups.every((g) => g.preRename === false));
});

test('groupCallers handles empty input', () => {
    assert.deepEqual(groupCallers(null), []);
    assert.deepEqual(groupCallers([]), []);
});

test('callerGroupLabel names each bucket', () => {
    assert.equal(callerGroupLabel({ preRename: true, depth: 1 }, 'RunHelp'), 'Still uses the old name "RunHelp"');
    assert.equal(callerGroupLabel({ preRename: false, depth: 1 }), 'Direct callers');
    assert.equal(callerGroupLabel({ preRename: false, depth: 3 }), '3 calls away');
});

test('callerGroupLabel stays generic without an old name', () => {
    assert.equal(callerGroupLabel({ preRename: true, depth: 1 }), 'Still uses the old name');
});

test('callerGroupLabel claims only what was measured', () => {
    // A grep hit for the old name can be a real call, a comment, or a string
    // literal - the label must not assert breakage we never verified.
    const label = callerGroupLabel({ preRename: true, depth: 1 }, 'RunHelp');
    assert.doesNotMatch(label, /break/i);
});

test('emptyCallGraphMessage ignores RenamedFrom - the chart is not rename-aware', () => {
    // Whether a symbol was renamed is a left-column concern (CallerGroup);
    // the chart's empty state must read identically either way, since it
    // only ever describes the real CALLS graph, never rename status.
    assert.equal(
        emptyCallGraphMessage({ RenamedFrom: 'RunHelp', Method: 'calls' }),
        'No callers in the dependency graph',
    );
});

test('emptyCallGraphMessage falls back to method and generic copy', () => {
    assert.equal(
        emptyCallGraphMessage({ Method: 'text-references' }),
        'Text reference method — no call graph available',
    );
    assert.equal(emptyCallGraphMessage({ Method: 'calls' }), 'No callers in the dependency graph');
    assert.equal(emptyCallGraphMessage(null), 'No callers in the dependency graph');
});
