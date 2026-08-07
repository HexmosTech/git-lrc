import { renderIcon } from './icons.js';

const { html } = window.preact;

// Small, non-blocking status line for the blast-radius report's upload to
// LiveReview (see internal/appcore/blastradius_bridge.go's uploadBlastRadiusReport).
// Renders nothing when idle/uploaded - only surfaces while in flight or on
// failure, since the upload never blocks or alters the rest of the review.
export function BlastUploadStatus({ upload }) {
    if (!upload || !upload.status || upload.status === 'idle' || upload.status === 'uploaded') {
        return '';
    }

    if (upload.status === 'uploading') {
        return html`
            <div class="blast-upload-status blast-upload-status--pending">
                <span class="blast-upload-spinner" aria-hidden="true"></span>
                Uploading blast-radius report to LiveReview…
            </div>
        `;
    }

    return html`
        <div class="blast-upload-status blast-upload-status--failed">
            ${renderIcon(html, 'issueWarning', { size: 14 })}
            Blast-radius report failed to upload — it won't be visible in LiveReview.
        </div>
    `;
}
