import {
    CONFIDENCE_ORDER,
    DEFAULT_SEVERITIES,
    FACET_FIELDS,
    buildCommentVisibilityKey,
    cloneIssueFilters,
    createDefaultIssueFilters,
    formatSeverityLabel,
    getNormalizedFacetValue,
    getSelectionFieldName,
    hasActiveIssueFilters,
    normalizeCommentShape,
    normalizeFacetValue,
    normalizeIssueFilters,
    sortValues,
    toggleIssueFilterValue,
} from './issue_filter_model.mjs';

function selectionMatches(disabledSet, normalizedValue) {
    if (!normalizedValue) {
        return true;
    }
    return !disabledSet.has(normalizedValue);
}

function matchesIssueFiltersExcludingField(comment, filters, excludedField) {
    const normalizedFilters = normalizeIssueFilters(filters);
    const shape = normalizeCommentShape(comment);

    return FACET_FIELDS.every((field) => {
        if (field === excludedField) {
            return true;
        }
        const selectionField = getSelectionFieldName(field);
        return selectionMatches(normalizedFilters.disabled[selectionField], getNormalizedFacetValue(shape, field));
    });
}

function matchesIssueFiltersExcludingFields(comment, filters, excludedFields) {
    const normalizedFilters = normalizeIssueFilters(filters);
    const excluded = new Set(excludedFields || []);
    const shape = normalizeCommentShape(comment);

    return FACET_FIELDS.every((field) => {
        if (excluded.has(field)) {
            return true;
        }
        const selectionField = getSelectionFieldName(field);
        return selectionMatches(normalizedFilters.disabled[selectionField], getNormalizedFacetValue(shape, field));
    });
}

function iterateIssueComments(files, visitor) {
    (files || []).forEach((file) => {
        const filePath = file?.FilePath || file?.file_path || file?.filePath || '';
        (file?.Hunks || []).forEach((hunk) => {
            (hunk?.Lines || []).forEach((line) => {
                if (!line?.IsComment || !Array.isArray(line?.Comments)) {
                    return;
                }
                line.Comments.forEach((comment) => visitor({ filePath, comment }));
            });
        });
    });
}

export {
    buildCommentVisibilityKey,
    cloneIssueFilters,
    createDefaultIssueFilters,
    DEFAULT_SEVERITIES,
    hasActiveIssueFilters,
    normalizeIssueFilters,
    toggleIssueFilterValue,
};

export function buildIssueFilterUniverse(files, hiddenCommentKeys) {
    const universe = {
        confidences: new Set(),
        types: new Set(),
        categories: new Set(),
        subcategories: new Set(),
    };

    iterateIssueComments(files, ({ filePath, comment }) => {
        if (hiddenCommentKeys instanceof Set && hiddenCommentKeys.has(buildCommentVisibilityKey(filePath, comment))) {
            return;
        }
        const shape = normalizeCommentShape(comment);
        if (shape.confidence) {
            universe.confidences.add(normalizeFacetValue(shape.confidence));
        }
        if (shape.type) {
            universe.types.add(normalizeFacetValue(shape.type));
        }
        if (shape.category) {
            universe.categories.add(normalizeFacetValue(shape.category));
        }
        if (shape.subcategory) {
            universe.subcategories.add(normalizeFacetValue(shape.subcategory));
        }
    });

    return {
        severities: DEFAULT_SEVERITIES,
        confidences: sortValues(universe.confidences, CONFIDENCE_ORDER),
        types: sortValues(universe.types),
        categories: sortValues(universe.categories),
        subcategories: sortValues(universe.subcategories),
    };
}

export function resetIssueFilters() {
    return createDefaultIssueFilters();
}

export function getCommentFilterValue(comment, field) {
    const shape = normalizeCommentShape(comment);
    return shape[field] || '';
}

export function matchesIssueFilters(comment, filters) {
    const normalized = normalizeIssueFilters(filters);
    const shape = normalizeCommentShape(comment);

    return FACET_FIELDS.every((field) => {
        const selectionField = getSelectionFieldName(field);
        return selectionMatches(normalized.disabled[selectionField], getNormalizedFacetValue(shape, field));
    });
}

export function getIssueFilterSummary(filters, universe = null) {
    const normalized = normalizeIssueFilters(filters);
    const active = [];
    const summaryFields = [
        ['severities', 'Severity', universe?.severities || DEFAULT_SEVERITIES],
        ['confidences', 'Confidence', universe?.confidences || []],
        ['types', 'Type', universe?.types || []],
        ['categories', 'Main Category', universe?.categories || []],
        ['subcategories', 'Subcategory', universe?.subcategories || []],
    ];

    summaryFields.forEach(([fieldName, label, allValues]) => {
        const disabled = normalized.disabled[fieldName];
        if (disabled.size === 0) {
            return;
        }
        const enabledCount = allValues.filter((value) => !disabled.has(value)).length;
        active.push(`${label}: ${enabledCount}`);
    });

    return active;
}

export function countFileVisibleIssues(file, filters, hiddenCommentKeys) {
    let visible = 0;
    iterateIssueComments([file], ({ filePath, comment }) => {
        if (hiddenCommentKeys instanceof Set && hiddenCommentKeys.has(buildCommentVisibilityKey(filePath, comment))) {
            return;
        }
        if (matchesIssueFilters(comment, filters)) {
            visible++;
        }
    });
    return visible;
}

export function countIssuesByFilters(files, filters, hiddenCommentKeys) {
    const severityCounts = {
        critical: 0,
        warning: 0,
        info: 0,
    };
    let total = 0;
    let visible = 0;

    iterateIssueComments(files, ({ filePath, comment }) => {
        const shape = normalizeCommentShape(comment);
        total++;
        severityCounts[shape.severity] = (severityCounts[shape.severity] || 0) + 1;
        if (hiddenCommentKeys instanceof Set && hiddenCommentKeys.has(buildCommentVisibilityKey(filePath, comment))) {
            return;
        }
        if (matchesIssueFilters(comment, filters)) {
            visible++;
        }
    });

    return {
        total,
        visible,
        severityCounts,
    };
}

export function buildIssueFacetOptions(files, filters, hiddenCommentKeys) {
    const normalized = normalizeIssueFilters(filters);
    const optionMaps = {
        severity: new Map(DEFAULT_SEVERITIES.map((value) => [value, { value, label: formatSeverityLabel(value), count: 0 }])),
        confidence: new Map(),
        type: new Map(),
        category: new Map(),
        subcategory: new Map(),
    };

    iterateIssueComments(files, ({ filePath, comment }) => {
        if (hiddenCommentKeys instanceof Set && hiddenCommentKeys.has(buildCommentVisibilityKey(filePath, comment))) {
            return;
        }
        const shape = normalizeCommentShape(comment);
        FACET_FIELDS.forEach((field) => {
            if (!matchesIssueFiltersExcludingField(comment, normalized, field)) {
                return;
            }
            const rawValue = shape[field];
            const normalizedValue = getNormalizedFacetValue(shape, field);
            if (!normalizedValue) {
                return;
            }
            const current = optionMaps[field].get(normalizedValue) || {
                value: normalizedValue,
                label: field === 'severity' ? formatSeverityLabel(normalizedValue) : rawValue,
                count: 0,
            };
            current.count += 1;
            optionMaps[field].set(normalizedValue, current);
        });
    });

    if (normalized.disabled.categories.size > 0) {
        const scopedSubcategories = new Map();
        iterateIssueComments(files, ({ filePath, comment }) => {
            if (hiddenCommentKeys instanceof Set && hiddenCommentKeys.has(buildCommentVisibilityKey(filePath, comment))) {
                return;
            }
            const shape = normalizeCommentShape(comment);
            if (!selectionMatches(normalized.disabled.categories, normalizeFacetValue(shape.category))) {
                return;
            }
            if (!shape.subcategory) {
                return;
            }
            const key = normalizeFacetValue(shape.subcategory);
            const current = optionMaps.subcategory.get(key) || {
                value: key,
                label: shape.subcategory,
                count: 0,
            };
            current.count = optionMaps.subcategory.get(key)?.count || current.count;
            scopedSubcategories.set(key, current);
        });
        optionMaps.subcategory = scopedSubcategories;
    }

    return {
        severities: sortValues(optionMaps.severity.keys(), DEFAULT_SEVERITIES).map((value) => ({
            ...optionMaps.severity.get(value),
            active: !normalized.disabled.severities.has(value),
        })),
        confidences: sortValues(optionMaps.confidence.keys(), CONFIDENCE_ORDER).map((value) => ({
            ...optionMaps.confidence.get(value),
            active: !normalized.disabled.confidences.has(value),
        })),
        types: sortValues(optionMaps.type.keys()).map((value) => ({
            ...optionMaps.type.get(value),
            active: !normalized.disabled.types.has(value),
        })),
        categories: sortValues(optionMaps.category.keys()).map((value) => ({
            ...optionMaps.category.get(value),
            active: !normalized.disabled.categories.has(value),
        })),
        subcategories: sortValues(optionMaps.subcategory.keys()).map((value) => ({
            ...optionMaps.subcategory.get(value),
            active: !normalized.disabled.subcategories.has(value),
        })),
    };
}

export function buildIssueCategoryGroups(files, filters, hiddenCommentKeys) {
    const normalized = normalizeIssueFilters(filters);
    const categoryMap = new Map();

    iterateIssueComments(files, ({ filePath, comment }) => {
        if (hiddenCommentKeys instanceof Set && hiddenCommentKeys.has(buildCommentVisibilityKey(filePath, comment))) {
            return;
        }
        if (!matchesIssueFiltersExcludingFields(comment, normalized, ['category', 'subcategory'])) {
            return;
        }

        const shape = normalizeCommentShape(comment);
        if (!shape.category) {
            return;
        }

        const categoryValue = normalizeFacetValue(shape.category);
        const categoryIsActive = !normalized.disabled.categories.has(categoryValue);
        const categoryEntry = categoryMap.get(categoryValue) || {
            value: categoryValue,
            label: shape.category,
            count: 0,
            active: categoryIsActive,
            subcategoryMap: new Map(),
        };
        categoryEntry.count += 1;

        if (shape.subcategory) {
            const subcategoryValue = normalizeFacetValue(shape.subcategory);
            const subcategoryIsActive = categoryIsActive && !normalized.disabled.subcategories.has(subcategoryValue);
            const subcategoryEntry = categoryEntry.subcategoryMap.get(subcategoryValue) || {
                value: subcategoryValue,
                label: shape.subcategory,
                count: 0,
                active: subcategoryIsActive,
            };
            subcategoryEntry.count += 1;
            categoryEntry.subcategoryMap.set(subcategoryValue, subcategoryEntry);
        }

        categoryMap.set(categoryValue, categoryEntry);
    });

    return sortValues(categoryMap.keys()).map((categoryValue) => {
        const entry = categoryMap.get(categoryValue);
        const subcategories = sortValues(entry.subcategoryMap.keys()).map((subcategoryValue) => entry.subcategoryMap.get(subcategoryValue));
        return {
            value: entry.value,
            label: entry.label,
            count: entry.count,
            active: entry.active,
            subcategories,
        };
    });
}

export function getIssueFilterStats(files, filters, hiddenCommentKeys, getVisibilityKey) {
    const normalized = normalizeIssueFilters(filters);
    const facetCounts = {
        category: new Map(),
    };
    const availableSubcategories = new Set();
    let total = 0;
    let visible = 0;

    iterateIssueComments(files, ({ filePath, comment }) => {
        const visibilityKey = typeof getVisibilityKey === 'function'
            ? getVisibilityKey(filePath, comment)
            : buildCommentVisibilityKey(filePath, comment);
        if (hiddenCommentKeys instanceof Set && hiddenCommentKeys.has(visibilityKey)) {
            total++;
            return;
        }

        const shape = normalizeCommentShape(comment);
        total++;
        if (shape.category) {
            facetCounts.category.set(shape.category, (facetCounts.category.get(shape.category) || 0) + 1);
        }
        if (matchesIssueFilters(comment, normalized)) {
            visible++;
        }
        if (shape.subcategory && selectionMatches(normalized.disabled.categories, normalizeFacetValue(shape.category))) {
            availableSubcategories.add(shape.subcategory);
        }
    });

    return {
        total,
        visible,
        facetCounts,
        availableSubcategories,
    };
}
