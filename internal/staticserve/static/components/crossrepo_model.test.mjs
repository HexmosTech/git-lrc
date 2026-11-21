import test from 'node:test';
import assert from 'node:assert/strict';

import {
    buildImpactHierarchy,
    crossRepoRouteLabel,
    findRepoMentions,
    groupRelatedRepos,
    hasImpactBranches,
    relatedRepoDisplayName,
    relatedRepoNameVariants,
} from './crossrepo_model.mjs';

const ENGINE_ID = 'root-public_lrdata-repo_cache-677-237-framework-examples__typescript__nestjs__blive-microservice-3-integration-repo';
const FRIENDLY = 'framework-examples/typescript/nestjs/blive-microservice-3-integration';

test('relatedRepoDisplayName prefers the friendly name', () => {
    assert.equal(relatedRepoDisplayName({ RelatedRepo: ENGINE_ID, RelatedRepoName: FRIENDLY }), FRIENDLY);
});

test('relatedRepoDisplayName decodes an engine project id', () => {
    assert.equal(relatedRepoDisplayName({ RelatedRepo: ENGINE_ID }), FRIENDLY);
});

test('relatedRepoDisplayName falls back to the provider URL path', () => {
    assert.equal(
        relatedRepoDisplayName({
            RelatedRepo: ENGINE_ID,
            RelatedRepoWebURL: `https://git.apps.hexmos.com/${FRIENDLY}`,
        }),
        FRIENDLY,
    );
});

test('relatedRepoNameVariants includes both the friendly and raw forms', () => {
    const variants = relatedRepoNameVariants({ RelatedRepo: ENGINE_ID, RelatedRepoName: FRIENDLY });
    assert.ok(variants.includes(ENGINE_ID));
    assert.ok(variants.includes(FRIENDLY));
});

test('crossRepoRouteLabel strips the engine route prefix', () => {
    assert.equal(crossRepoRouteLabel('__route__POST__/v1/e-kyc/read-aadhaar'), '/v1/e-kyc/read-aadhaar');
    assert.equal(crossRepoRouteLabel('/already/a/path'), '/already/a/path');
});

test('groupRelatedRepos groups by id and dedupes routes', () => {
    const groups = groupRelatedRepos([
        { RelatedRepoID: 2203, RelatedRepoName: FRIENDLY, CallSite: '__route__POST__/v1/a', MatchKind: 'exact' },
        { RelatedRepoID: 2203, RelatedRepoName: FRIENDLY, CallSite: '__route__POST__/v1/a', MatchKind: 'version-alias' },
        { RelatedRepoID: 2203, RelatedRepoName: FRIENDLY, CallSite: '__route__POST__/v1/b' },
    ]);
    assert.equal(groups.length, 1);
    assert.equal(groups[0].name, FRIENDLY);
    assert.deepEqual(groups[0].callSites, ['__route__POST__/v1/a', '__route__POST__/v1/b']);
    assert.deepEqual(groups[0].matchKinds.sort(), ['exact', 'version-alias']);
});

test('groupRelatedRepos separates distinct repos', () => {
    const groups = groupRelatedRepos([
        { RelatedRepoID: 1, RelatedRepoName: 'org/a', CallSite: 'r1' },
        { RelatedRepoID: 2, RelatedRepoName: 'org/b', CallSite: 'r2' },
    ]);
    assert.equal(groups.length, 2);
    assert.deepEqual(groups.map((g) => g.name).sort(), ['org/a', 'org/b']);
});

test('buildImpactHierarchy keeps callers and appends a related repo branch', () => {
    const symbol = {
        Name: 'readAadhaar',
        QualifiedName: 'local.consumer.EKycService.readAadhaar',
        Callers: [{ QualifiedName: 'local.consumer.Controller.read', Depth: 1, Weight: 1, Path: [] }],
        ImpactedRelatedRepos: [{
            RelatedRepoID: 2203,
            RelatedRepoName: FRIENDLY,
            RelatedRepoWebURL: `https://git.apps.hexmos.com/${FRIENDLY}`,
            CallSite: '__route__POST__/v1/e-kyc/read-aadhaar',
            MatchKind: 'exact',
        }],
    };
    const root = buildImpactHierarchy(symbol);
    assert.equal(root.children.length, 2);
    const branch = root.children.find((c) => c.isRelatedRepo);
    assert.ok(branch);
    assert.equal(branch.name, FRIENDLY);
    assert.equal(branch.children.length, 1);
    assert.equal(branch.children[0].name, '/v1/e-kyc/read-aadhaar');
    assert.equal(branch.children[0].isRelatedRoute, true);
});

test('buildImpactHierarchy draws a related-only symbol and dedupes chains', () => {
    const callSite = '__route__POST__/v1/e-kyc/read-aadhaar';
    const symbol = {
        Name: 'readAadhaar',
        QualifiedName: 'local.consumer.EKycService.readAadhaar',
        Callers: [],
        ImpactedRelatedRepos: [{ RelatedRepoID: 2203, RelatedRepoName: FRIENDLY, CallSite: callSite }],
        ImpactedRepoChains: [{ Hops: [
            { RelatedRepoID: 2203, RelatedRepoName: FRIENDLY, CallSite: callSite },
            { RelatedRepoID: 2202, RelatedRepoName: 'org/common', CallSite: '__route__POST__/v1/common' },
        ] }],
    };
    const root = buildImpactHierarchy(symbol);
    assert.equal(root.children.length, 2);
    assert.equal(root.children.find((c) => c.name === FRIENDLY).children.length, 1);
    assert.ok(hasImpactBranches(symbol));
    assert.equal(hasImpactBranches({ Callers: [] }), false);
});

test('findRepoMentions matches both friendly and engine-id text', () => {
    const impacts = [{ RelatedRepoID: 2203, RelatedRepo: ENGINE_ID, RelatedRepoName: FRIENDLY, RelatedRepoWebURL: 'https://git/x' }];
    const friendlyText = findRepoMentions(`Calls into ${FRIENDLY}`, impacts);
    assert.equal(friendlyText.length, 1);
    assert.equal(friendlyText[0].display, FRIENDLY);
    const rawText = findRepoMentions(`Calls into ${ENGINE_ID}`, impacts);
    assert.equal(rawText.length, 1);
    assert.equal(rawText[0].display, FRIENDLY);
});
