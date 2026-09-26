/**
 * KRn for pi — keeps repository evidence small and cache-friendly on local models.
 *
 * Three independent parts, each toggled by an environment variable. Only the map is on by
 * default: in the local ablation (README, "Local models with pi") the map alone matched all
 * three parts, and the model almost never called find_code. Set "1" to enable, "0" to disable.
 *   KRN_PI_BOUND  bound oversized grep/find/ls/bash search output to a projection whose
 *                 full text is saved under .git/krn/runs; on zero hits, append `krn find`.
 *   KRN_PI_TOOL   register `find_code`: ranked file:line hits with context from `krn find`.
 *   KRN_PI_MAP    on the first prompt of a session, append a query-focused `krn map` as a
 *                 message (never a system-prompt edit, so the KV prefix cache survives).
 *
 * Every part fails open: outside a Git repository or when `krn` errors, pi behaves as usual.
 */
import * as fs from "node:fs";
import * as path from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const KRN = process.env.KRN_BIN || "krn";
const on = (name: string, byDefault: boolean) => (process.env[name] ?? (byDefault ? "1" : "0")) !== "0";

// ~4 bytes per token: 6000 bytes is ~1.5k tokens, about 3 s of cold prefill on an M4 Pro.
const BOUND_BYTES = Number(process.env.KRN_PI_BOUND_BYTES || 6000);
const BOUND_KEEP_LINES = 40;
const SEARCH_TOOLS = new Set(["grep", "find", "ls"]);
const BASH_SEARCH = /^\s*(grep|rg|egrep|fgrep|find|ls\s+-[a-zA-Z]*R|git\s+grep|ag|fd)\b/;

export default function (pi: ExtensionAPI) {
	let gitDir: string | null | undefined;
	let mapped = false;

	async function repoGitDir(cwd: string, signal?: AbortSignal): Promise<string | null> {
		if (gitDir !== undefined) return gitDir;
		const r = await pi.exec("git", ["-C", cwd, "rev-parse", "--absolute-git-dir"], { signal, timeout: 3000 });
		gitDir = r.code === 0 ? r.stdout.trim() : null;
		return gitDir;
	}

	async function krn(args: string[], cwd: string, signal?: AbortSignal, timeout = 15000): Promise<string | null> {
		try {
			const r = await pi.exec(KRN, args, { signal, timeout, cwd });
			return r.code === 0 && r.stdout.trim() ? r.stdout : null;
		} catch {
			return null;
		}
	}

	if (on("KRN_PI_TOOL", false)) {
		pi.registerTool({
			name: "find_code",
			label: "Find code",
			description:
				"Locate code by behavior or names in the current Git repository. Accepts plain words or identifiers " +
				"(e.g. 'rest timer end time', 'saveSession'). Returns the best files ranked, each hit as line number, " +
				"enclosing function, and 2 lines of context. Faster and smaller than grep followed by reading files.",
			promptSnippet: "Locate code by behavior or identifiers; ranked file:line hits with context",
			promptGuidelines: [
				"Use find_code before grep/find/read when you do not yet know which file implements something.",
			],
			parameters: Type.Object({
				query: Type.String({ description: "What to find: words describing the behavior and/or identifiers" }),
				max_files: Type.Optional(Type.Number({ description: "Maximum files to return (default 5)" })),
			}),
			async execute(_id, params, signal, _onUpdate, ctx) {
				const n = Math.max(1, Math.min(10, Math.floor(params.max_files ?? 5)));
				const out = await krn(["find", "--max-files", String(n), params.query], ctx.cwd, signal);
				return {
					content: [{ type: "text", text: out ?? "find_code unavailable here (not a Git repository or krn failed); use grep." }],
					details: {},
				};
			},
		});
	}

	if (on("KRN_PI_BOUND", false)) {
		pi.on("tool_result", async (event, ctx) => {
			if (event.isError) return;
			const input = (event.input ?? {}) as Record<string, unknown>;
			const isSearchTool = SEARCH_TOOLS.has(event.toolName);
			const command = event.toolName === "bash" ? String(input.command ?? "") : "";
			if (!isSearchTool && !BASH_SEARCH.test(command)) return;
			const text = (event.content ?? [])
				.filter((c: any) => c.type === "text")
				.map((c: any) => c.text as string)
				.join("\n");

			if (isEmptySearch(event.toolName, text)) {
				const query = event.toolName === "bash" ? queryFromCommand(command) : String(input.pattern ?? "");
				const words = query.replace(/\\[bBsSwWdD]|[^A-Za-z0-9_$]+/g, " ").trim();
				if (!words) return;
				const hits = await krn(["find", "--max-files", "3", "--budget", "1200", words], ctx.cwd, ctx.signal);
				if (!hits || hits.startsWith("no matches")) return;
				return { content: [{ type: "text", text: `${text.trim()}\n\nkrn find for the same words:\n${hits}` }] };
			}

			if (Buffer.byteLength(text) <= BOUND_BYTES) return;
			const dir = await repoGitDir(ctx.cwd, ctx.signal);
			let saved = "";
			if (dir) {
				try {
					const runs = path.join(dir, "krn", "runs");
					fs.mkdirSync(runs, { recursive: true, mode: 0o700 });
					saved = path.join(runs, `${new Date().toISOString().replace(/[:.]/g, "")}-pi-${event.toolName}.log`);
					fs.writeFileSync(saved, text, { mode: 0o600 });
				} catch {
					saved = "";
				}
			}
			const lines = text.split("\n");
			let kept = lines.slice(0, BOUND_KEEP_LINES).join("\n");
			if (Buffer.byteLength(kept) > BOUND_BYTES) kept = kept.slice(0, BOUND_BYTES);
			const tail =
				`\n… bounded: showing ${Math.min(BOUND_KEEP_LINES, lines.length)} of ${lines.length} lines` +
				(saved ? `; full output saved at ${saved}` : "") +
				". Narrow the search (path, glob, more specific pattern) or use find_code.";
			return { content: [{ type: "text", text: kept + tail }] };
		});
	}

	if (on("KRN_PI_MAP", true)) {
		pi.on("before_agent_start", async (event, ctx) => {
			if (mapped) return;
			mapped = true;
			const prompt = String(event.prompt ?? "");
			// A prompt that already names a file needs no orientation.
			if (/[\w./-]+\.[a-z]{1,5}(:\d+)?\b/i.test(prompt) && /[\\/]/.test(prompt)) return;
			if (!(await repoGitDir(ctx.cwd))) return;
			const map = await krn(["map", "--tokens", process.env.KRN_PI_MAP_TOKENS || "800", "--focus", prompt], ctx.cwd, undefined, 20000);
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
}

function isEmptySearch(tool: string, text: string): boolean {
	const t = text.trim();
	if (!t) return true;
	if (tool === "bash") return false; // an empty bash result is already caught above
	return /^(no (files|matches) found|no matches)/i.test(t);
}

function queryFromCommand(command: string): string {
	// Take quoted or bare pattern arguments, skipping flags and paths.
	const quoted = [...command.matchAll(/(['"])(.*?)\1/g)].map((m) => m[2]);
	if (quoted.length) return quoted.join(" ");
	return command
		.split(/\s+/)
		.slice(1)
		.filter((a) => a && !a.startsWith("-") && !a.includes("/") && !a.includes("."))
		.join(" ");
}
