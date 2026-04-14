export function planLabel(planCode) {
    const normalized = String(planCode || '').trim().toLowerCase();
    if (!normalized) return 'Plan';
    if (normalized === 'free_30k' || normalized === 'free') return 'Free 30k';
    if (normalized === 'team_32usd' || normalized === 'team') return 'Team 100k';
    if (normalized === 'loc_100k') return 'Team 100k';
    if (normalized === 'loc_200k') return 'Team 200k';
    if (normalized === 'loc_400k') return 'Team 400k';
    if (normalized === 'loc_800k') return 'Team 800k';
    if (normalized === 'loc_1600k') return 'Team 1.6M';
    if (normalized === 'loc_3200k') return 'Team 3.2M';
    return planCode;
}

export function clampUsagePercent(value) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) return 0;
    if (parsed < 0) return 0;
    if (parsed > 100) return 100;
    return Math.round(parsed);
}

export function normalizeUsagePayload(raw, fallbackReason = 'Usage data unavailable right now.') {
    const source = raw && typeof raw === 'object' ? raw : {};
    return {
        available: Boolean(source.available),
        unavailableReason: String(source.unavailable_reason || fallbackReason),
        planCode: String(source.plan_code || '').trim(),
        usagePct: clampUsagePercent(source.usage_pct),
        customerState: String(source.customer_state || '').trim().toLowerCase(),
        blocked: Boolean(source.blocked),
        locUsed: Number.isFinite(Number(source.loc_used)) ? Number(source.loc_used) : 0,
        locLimit: Number.isFinite(Number(source.loc_limit)) ? Number(source.loc_limit) : 0,
        resetAt: String(source.reset_at || '').trim(),
        myUsageLOC: Number.isFinite(Number(source.my_usage_loc)) ? Number(source.my_usage_loc) : 0,
        myOperationCount: Number.isFinite(Number(source.my_operation_count)) ? Number(source.my_operation_count) : 0,
        mySharePct: Number.isFinite(Number(source.my_share_pct)) ? Number(source.my_share_pct) : 0,
        topMembers: Array.isArray(source.top_members)
            ? source.top_members.map((member) => ({
                label: String(member?.label || '').trim(),
                loc: Number.isFinite(Number(member?.loc)) ? Number(member.loc) : 0,
                share: Number.isFinite(Number(member?.share)) ? Number(member.share) : 0,
                kind: String(member?.kind || '').trim(),
            }))
            : [],
        canViewTeamBreakdown: Boolean(source.can_view_team_breakdown),
        cloudURL: String(source.cloud_url || '').trim(),
        fetchedAt: String(source.fetched_at || '').trim(),
    };
}

export function usageTone(chip, loading) {
    if (loading) return 'loading';
    if (!chip || !chip.available) return 'unavailable';
    if (chip.blocked || chip.customerState === 'action_needed' || chip.customerState === 'payment_failed') return 'critical';
    if (chip.usagePct >= 80) return 'warn';
    return 'ok';
}

export function formatResetAt(value) {
    const raw = String(value || '').trim();
    if (!raw) return 'Not available';
    const parsed = new Date(raw);
    if (Number.isNaN(parsed.getTime())) return 'Not available';
    return new Intl.DateTimeFormat(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
    }).format(parsed);
}
