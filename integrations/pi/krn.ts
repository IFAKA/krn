/**
 * KRn for pi — on the first prompt of a session, append a query-focused `krn map` as a
 * message (never a system-prompt edit, so the KV prefix cache survives).
 *
 * Only the map is shipped: in the local ablation (docs/benchmarks.md) it alone matched the map
 * plus bounded search output plus a `find_code` tool, and those two parts were removed.
 *   KRN_PI_MAP         "0" disables the map.
 *   KRN_PI_MAP_TOKENS  map token budget (default 800).
 *
 * Fails open: outside a Git repository or when `krn` errors, pi behaves as usual.
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const KRN = process.env.KRN_BIN || "krn";

export default function (pi: ExtensionAPI) {
	if (process.env.KRN_PI_MAP === "0") return;
	let mapped = false;

	pi.on("before_agent_start", async (event, ctx) => {
		if (mapped) return;
		mapped = true;
		const prompt = String(event.prompt ?? "");
		// A prompt that already names a file needs no orientation.
		if (/[\w./-]+\.[a-z]{1,5}(:\d+)?\b/i.test(prompt) && /[\\/]/.test(prompt)) return;
		const git = await pi.exec("git", ["-C", ctx.cwd, "rev-parse", "--absolute-git-dir"], { timeout: 3000 });
		if (git.code !== 0) return;
		let map: string | null = null;
		try {
			const r = await pi.exec(KRN, ["map", "--tokens", process.env.KRN_PI_MAP_TOKENS || "800", "--focus", prompt], {
				timeout: 20000,
				cwd: ctx.cwd,
			});
			map = r.code === 0 && r.stdout.trim() ? r.stdout : null;
		} catch {
			map = null;
		}
		if (!map) return;
		return {
			message: {
				customType: "krn-map",
				content: `Repository map (ranked by relevance to the request; signatures only):\n${map}`,
				display: false,
			},
		};
	});
}
