import test from 'node:test';
import assert from 'node:assert/strict';

import {
    buildHierarchy,
    callerGroupLabel,
    callersForRenameView,
    emptyCallGraphMessage,
    groupCallers,
    hasRenameViews,
    RENAME_VIEW_AFTER,
    RENAME_VIEW_BEFORE,
    shortName,
    symbolForRenameView,
} from './callgraph_model.mjs';

const RENAMED = {
    Name: 'RuHelp',
    QualifiedName: 'pkg.RuHelp',
    RenamedFrom: 'RunHelp',
    Callers: [
        { QualifiedName: 'pkg.main', Depth: 1, PreRename: true },
        { QualifiedName: 'pkg.migrated', Depth: 1 },
    ],
};

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

test('buildHierarchy marks pre-rename callers', () => {
    const root = buildHierarchy({
        Name: 'RuHelp',
        QualifiedName: 'pkg.RuHelp',
        Callers: [{ QualifiedName: 'pkg.main', Depth: 1, Weight: 1, PreRename: true }],
    });

    assert.equal(root.children.length, 1);
    assert.equal(root.children[0].qualifiedName, 'pkg.main');
    assert.equal(root.children[0].preRename, true);
});

test('buildHierarchy propagates preRename across a whole branch', () => {
    // An intermediate only reaches the renamed symbol through the broken
    // name, so it is breaking too - not just the leaf at the end.
    const root = buildHierarchy({
        Name: 'RuHelp',
        QualifiedName: 'pkg.RuHelp',
        Callers: [
            { QualifiedName: 'pkg.direct', Depth: 1, Weight: 1, PreRename: true },
            { QualifiedName: 'pkg.top', Depth: 2, Weight: 0.5, Path: ['pkg.direct'], PreRename: true },
        ],
    });

    const via = root.children[0];
    assert.equal(via.preRename, true, 'intermediate must inherit the breaking flag');
    assert.equal(via.children[0].preRename, true);
});

test('buildHierarchy leaves live callers unflagged', () => {
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

test('emptyCallGraphMessage explains a fully-migrated rename', () => {
    assert.equal(
        emptyCallGraphMessage({ RenamedFrom: 'RunHelp', Method: 'calls' }),
        'Renamed from "RunHelp" — nothing else uses the old name',
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

test('emptyCallGraphMessage is view-specific for a renamed symbol', () => {
    assert.equal(
        emptyCallGraphMessage(RENAMED, RENAME_VIEW_AFTER),
        'Nothing calls "RuHelp" yet — renamed from "RunHelp"',
    );
    assert.equal(
        emptyCallGraphMessage(RENAMED, RENAME_VIEW_BEFORE),
        'Nothing else uses the old name "RunHelp"',
    );
});

test('hasRenameViews is true only for renamed symbols', () => {
    assert.equal(hasRenameViews(RENAMED), true);
    assert.equal(hasRenameViews({ QualifiedName: 'pkg.leftWidth', Callers: [] }), false);
    assert.equal(hasRenameViews(null), false);
});

test('callersForRenameView splits the two graphs', () => {
    assert.deepEqual(
        callersForRenameView(RENAMED, RENAME_VIEW_BEFORE).map((c) => c.QualifiedName),
        ['pkg.main'],
    );
    assert.deepEqual(
        callersForRenameView(RENAMED, RENAME_VIEW_AFTER).map((c) => c.QualifiedName),
        ['pkg.migrated'],
    );
});

test('callersForRenameView ignores the view for unrenamed symbols', () => {
    // A symbol that was not renamed has one graph, not two - the view must
    // not silently filter its callers away.
    const plain = { QualifiedName: 'pkg.leftWidth', Callers: [{ QualifiedName: 'pkg.a', Depth: 1 }] };
    assert.equal(callersForRenameView(plain, RENAME_VIEW_BEFORE).length, 1);
    assert.equal(callersForRenameView(plain, RENAME_VIEW_AFTER).length, 1);
});

test('symbolForRenameView narrows Callers but keeps identity fields', () => {
    const before = symbolForRenameView(RENAMED, RENAME_VIEW_BEFORE);
    assert.equal(before.QualifiedName, 'pkg.RuHelp');
    assert.equal(before.RenamedFrom, 'RunHelp');
    assert.deepEqual(before.Callers.map((c) => c.QualifiedName), ['pkg.main']);
});

test('symbolForRenameView returns unrenamed symbols unchanged by identity', () => {
    // Charts key their render effect on symbol identity; returning a fresh
    // object for every unrenamed symbol would redraw on each parent render.
    const plain = { QualifiedName: 'pkg.leftWidth', Callers: [] };
    assert.equal(symbolForRenameView(plain, RENAME_VIEW_AFTER), plain);
});
