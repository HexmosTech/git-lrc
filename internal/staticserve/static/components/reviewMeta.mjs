// Module-level store for review metadata (apiURL, reviewID) set once on load.
let _meta = { apiURL: '', reviewID: '' };
export function setReviewMeta(meta) { Object.assign(_meta, meta); }
export function getReviewMeta() { return _meta; }
