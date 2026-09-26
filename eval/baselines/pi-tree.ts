/**
 * Baseline for `krn eval-pi` variant T, not part of KRn: what a user gets without KRn by
 * pasting the repository's file list into the first prompt. Same delivery as the map
 * (appended message on the first prompt, skipped when the prompt names a file path) and
 * the same byte budget (KRN_PI_MAP_TOKENS × 4), but `git ls-files` order, no ranking,
 * no signatures. Keep it in step with fileList in internal/eval/claude.go.
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
	let done = false;
	pi.on("before_agent_start", async (event, ctx) => {
		if (done) return;
		done = true;
		const prompt = String(event.prompt ?? "");
		if (/[\w./-]+\.[a-z]{1,5}(:\d+)?\b/i.test(prompt) && /[\\/]/.test(prompt)) return;
		const r = await pi.exec("git", ["-C", ctx.cwd, "ls-files"], { timeout: 5000 });
		if (r.code !== 0 || !r.stdout.trim()) return;
		const budget = 4 * Number(process.env.KRN_PI_MAP_TOKENS || 800);
		const files = r.stdout.trim().split("\n");
		let text = "";
		let n = 0;
		for (const f of files) {
			if (Buffer.byteLength(text + f + "\n") > budget) break;
			text += f + "\n";
			n++;
		}
		if (n < files.length) text += `… ${files.length - n} more files\n`;
		return {
			message: { customType: "file-list", content: `Repository files (git ls-files):\n${text}`, display: false },
		};
	});
}
