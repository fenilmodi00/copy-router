/** Maps internal routing reason strings to short playground labels. */
export function formatPlaygroundReason(reason: string, model?: string): string {
	if (!reason) {
		return model ? `Routed to ${model}` : "Auto-routed";
	}
	if (reason.startsWith("cluster:")) {
		return "Auto-routed";
	}
	return reason;
}
