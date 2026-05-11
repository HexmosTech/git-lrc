export function shouldShowAllClear({ status, totalComments, errorSummary, summarySlidesEligibility }) {
    const normalizedStatus = typeof status === 'string' ? status.trim().toLowerCase() : '';
    const parsedComments = Number(totalComments);
    const normalizedComments = Number.isFinite(parsedComments) ? parsedComments : 0;
    const hasError = typeof errorSummary === 'string'
        ? errorSummary.trim() !== ''
        : errorSummary != null;
    const hasStructuredSummary = Boolean(summarySlidesEligibility && summarySlidesEligibility.eligible === true);

    return normalizedStatus === 'completed' && !hasError && normalizedComments === 0 && hasStructuredSummary;
}
