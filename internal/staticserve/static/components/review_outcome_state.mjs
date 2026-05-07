export function shouldShowAllClear({ status, totalComments, errorSummary }) {
    const normalizedStatus = typeof status === 'string' ? status.trim().toLowerCase() : '';
    const parsedComments = Number(totalComments);
    const normalizedComments = Number.isFinite(parsedComments) ? parsedComments : 0;
    const hasError = typeof errorSummary === 'string'
        ? errorSummary.trim() !== ''
        : errorSummary != null;

    return normalizedStatus === 'completed' && !hasError && normalizedComments === 0;
}
