<script lang="ts">
	import QRCode from "qrcode";
	import { onDestroy } from "svelte";

	type Status = "DISABLED" | "NOT_PAIRED" | "PAIRED" | "READY" | "WAITING" | "ERROR";
	type Group = { jid: string; name: string };

	let apiBase = $state(typeof window !== "undefined" ? window.location.origin : "");
	let token = $state("");
	let showToken = $state(false);
	let status = $state<Status | "IDLE">("IDLE");
	let qrCode = $state("");
	let qrDataUrl = $state("");
	let groups = $state<Group[]>([]);
	let message = $state("");
	let error = $state("");
	let loading = $state(false);
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	const curlQr = $derived.by(() => {
		const base = apiBase.replace(/\/$/, "") || "http://localhost:8080";
		return `CODE=$(curl -s -H "Authorization: Bearer $TOKEN" \\
  ${base}/api/v1/whatsapp/qr \\
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("qr",""))')
npx -y qrcode@1.5.4 -o /tmp/whatsapp-qr.png "$CODE" && open /tmp/whatsapp-qr.png`;
	});

	function authHeaders(): HeadersInit {
		return { Authorization: `Bearer ${token.trim()}` };
	}

	function baseUrl() {
		return apiBase.replace(/\/$/, "");
	}

	function stopPolling() {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
	}

	onDestroy(stopPolling);

	async function apiGet(path: string) {
		const res = await fetch(`${baseUrl()}${path}`, { headers: authHeaders() });
		const body = await res.json().catch(() => ({}));
		if (!res.ok) {
			const msg = body?.error?.message || body?.message || `Request failed (${res.status})`;
			throw new Error(msg);
		}
		return body;
	}

	async function renderQr(code: string) {
		if (!code) {
			qrDataUrl = "";
			return;
		}
		qrDataUrl = await QRCode.toDataURL(code, { margin: 1, width: 280, color: { dark: "#11151d", light: "#ffffff" } });
	}

	async function refreshStatus() {
		error = "";
		try {
			const body = await apiGet("/api/v1/whatsapp/status");
			const s = String(body?.status || "DISABLED");
			if (s === "DISABLED") {
				status = "DISABLED";
				message = "WhatsApp bot is disabled on the backend (WHATSAPP_ENABLED=false).";
				stopPolling();
				return;
			}
			if (body?.is_logged_in) {
				status = "PAIRED";
				message = "Bot is paired. You can set up groups from the bot's phone.";
				stopPolling();
				await loadGroups();
				return;
			}
			message = `Connected: ${body?.is_connected ? "yes" : "no"} · Logged in: no`;
			if (status !== "READY" && status !== "WAITING") status = "NOT_PAIRED";
		} catch (e) {
			status = "ERROR";
			error = e instanceof Error ? e.message : "Failed to load status.";
			stopPolling();
		}
	}

	async function loadGroups() {
		try {
			const body = await apiGet("/api/v1/whatsapp/groups");
			groups = Array.isArray(body?.groups) ? body.groups : [];
		} catch {
			groups = [];
		}
	}

	async function fetchQrOnce() {
		const body = await apiGet("/api/v1/whatsapp/qr");
		const s = String(body?.status || "");
		if (s === "PAIRED") {
			status = "PAIRED";
			qrCode = "";
			await renderQr("");
			message = "Bot is paired.";
			stopPolling();
			await loadGroups();
			return;
		}
		if (s === "READY" && body?.qr) {
			status = "READY";
			qrCode = String(body.qr);
			await renderQr(qrCode);
			message = "Scan this QR with the bot phone: WhatsApp → Settings → Linked devices → Link a device.";
			return;
		}
		status = "WAITING";
		message = "Waiting for pairing code... retrying automatically.";
	}

	async function startPairing() {
		if (!token.trim()) {
			error = "Host token is required.";
			return;
		}
		loading = true;
		error = "";
		message = "";
		qrCode = "";
		await renderQr("");
		groups = [];
		stopPolling();
		try {
			await refreshStatus();
			if (status === "PAIRED" || status === "DISABLED") return;
			await fetchQrOnce();
			pollTimer = setInterval(async () => {
				try {
					await fetchQrOnce();
				} catch (e) {
					error = e instanceof Error ? e.message : "Failed to fetch QR.";
					stopPolling();
				}
			}, 3000);
		} catch (e) {
			status = "ERROR";
			error = e instanceof Error ? e.message : "Failed to start pairing.";
		} finally {
			loading = false;
		}
	}

	function clearSensitive() {
		stopPolling();
		token = "";
		qrCode = "";
		qrDataUrl = "";
		groups = [];
		message = "Token cleared from this page.";
		error = "";
		status = "IDLE";
	}
</script>

<svelte:head>
	<title>Bot setup — Deuce</title>
	<meta name="description" content="Pair the Deuce WhatsApp bot with a host token." />
</svelte:head>

<div class="container-wide page">
	<p class="eyebrow">Operator setup</p>
	<h1>Pair WhatsApp bot</h1>
	<p class="lead muted">
		Use your host token to fetch a pairing QR from the backend. Token stays in memory only on this page.
		<code>curl</code> remains the backup for headless/CORS/scripted setup.
	</p>

	<div class="grid">
		<section class="card">
			<h2>Connect</h2>
			<div class="field">
				<label for="api-base">Backend API base URL</label>
				<input id="api-base" class="input" bind:value={apiBase} placeholder="http://localhost:8080" autocomplete="off" />
				<p class="hint">Use your backend origin. Browser requests need it allowed by backend <code>CORS_ORIGINS</code>.</p>
			</div>
			<div class="field">
				<label for="token">Host token (Bearer)</label>
				<div class="token-row">
					<input id="token" class="input" type={showToken ? "text" : "password"} bind:value={token} placeholder="Paste host token" autocomplete="off" />
					<button class="btn btn-ghost btn-sm" type="button" onclick={() => (showToken = !showToken)}>{showToken ? "Hide" : "Show"}</button>
				</div>
				<p class="hint">Sensitive: grants host API access. Don't use on shared devices; clear when done.</p>
			</div>
			<div class="actions">
				<button class="btn btn-primary" type="button" onclick={startPairing} disabled={loading || !token.trim()}>
					{loading ? "Connecting..." : "Get pairing QR"}
				</button>
				<button class="btn btn-ghost" type="button" onclick={refreshStatus} disabled={loading || !token.trim()}>Refresh status</button>
				<button class="btn btn-ghost" type="button" onclick={clearSensitive}>Clear</button>
			</div>

			{#if error}<p class="msg err">{error}</p>{/if}
			{#if message}<p class="msg">{message}</p>{/if}

			<div class="status-line">
				<span class="label">Status</span>
				<span class={`status-pill s-${status.toLowerCase()}`}>{status}</span>
			</div>

			{#if qrDataUrl}
				<div class="qr-wrap">
					<img src={qrDataUrl} alt="WhatsApp pairing QR code" width="280" height="280" />
					<p class="muted">Code is short-lived and rotates. Keep it private.</p>
				</div>
			{:else if qrCode}
				<pre><code>{qrCode}</code></pre>
			{/if}

			{#if groups.length}
				<h3>Joined groups</h3>
				<table>
					<thead><tr><th>Name</th><th>JID</th></tr></thead>
					<tbody>
						{#each groups as g}
							<tr><td>{g.name}</td><td><code>{g.jid}</code></td></tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</section>

		<section class="card">
			<h2>What this page can/can't do</h2>
			<ul class="list">
				<li><strong>Can:</strong> check WhatsApp status, fetch pairing QR, list joined groups (after paired).</li>
				<li><strong>Can't:</strong> run <code>setup admin</code> / <code>setup announce</code>. Those must be sent from the <strong>bot's own phone</strong> in WhatsApp.</li>
			</ul>

			<h3>After pairing</h3>
			<ol class="list">
				<li>On bot phone, in admin group send: <code>setup admin &lt;club name&gt;</code></li>
				<li>On bot phone, in players' group send: <code>setup announce</code></li>
				<li>Confirm with <code>setup status</code> or refresh status here.</li>
			</ol>

			<h3>curl backup</h3>
			<p class="muted">Use when browser/CORS is inconvenient or on a trusted machine only.</p>
			<pre><code>{curlQr}</code></pre>

			<div class="note">
				<strong>Security notes:</strong> token is not stored; pairing code expires quickly. Use HTTPS for non-local backends, don't screenshot/share QR/token, and clear this page when done.
			</div>
		</section>
	</div>
</div>

<style>
	.page { padding: 28px 0 56px; }
	.eyebrow { color: var(--accent); font-weight: 600; font-size: .9rem; margin-bottom: 8px; }
	h1 { font-size: clamp(1.8rem, 3.6vw, 2.8rem); letter-spacing: -0.04em; margin: 0 0 10px; }
	.lead { max-width: 70ch; margin-bottom: 20px; line-height: 1.6; }
	.grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 14px; align-items: start; }
	h2 { font-size: 1.2rem; margin: 0 0 14px; }
	h3 { font-size: 1rem; margin: 18px 0 8px; }
	.field { margin-bottom: 14px; }
	label { display:block; font-size:.85rem; color: var(--text-dim); font-weight:600; margin-bottom:6px; }
	.input { width: 100%; }
	.hint { color: var(--text-faint); font-size: .82rem; margin-top: 6px; line-height: 1.45; }
	.token-row { display:flex; gap:8px; }
	.token-row .input { flex:1; }
	.actions { display:flex; flex-wrap:wrap; gap:8px; margin: 14px 0; }
	.msg { margin: 10px 0; font-size: .92rem; color: var(--text-dim); }
	.msg.err { color: var(--danger); }
	.status-line { display:flex; align-items:center; gap:10px; margin: 14px 0; font-size: .92rem; }
	.label { color: var(--text-dim); }
	.status-pill { padding: 4px 10px; border-radius: 999px; font-size: .78rem; font-weight: 700; border: 1px solid var(--border); background: var(--bg-elevated-2); }
	.s-ready, .s-paired { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 35%, var(--border)); }
	.s-waiting { color: var(--warn); }
	.s-error, .s-disabled { color: var(--danger); }
	.qr-wrap { margin-top: 14px; padding: 14px; border: 1px solid var(--border-soft); border-radius: 14px; background: #fff; color: #111; width: fit-content; max-width: 100%; }
	.qr-wrap img { display:block; max-width:100%; height:auto; }
	.qr-wrap p { color: #444; font-size: .82rem; margin-top: 8px; }
	.list { margin: 0; padding-left: 18px; display:grid; gap: 8px; color: var(--text-dim); }
	.list strong { color: var(--text); }
	.note { margin-top: 16px; padding: 14px; border-radius: 12px; border: 1px solid color-mix(in srgb, var(--warn) 35%, var(--border)); background: color-mix(in srgb, var(--warn) 8%, transparent); color: var(--text-dim); font-size: .9rem; line-height: 1.5; }
	pre { background:#0f1115; border:1px solid var(--border-soft); border-radius:14px; padding:14px; overflow-x:auto; margin:12px 0; }
	code { font-family: var(--font-mono); font-size: .88em; }
	:not(pre)>code { background: var(--bg-elevated-2); border:1px solid var(--border-soft); border-radius:6px; padding:1px 6px; }
	table { width:100%; border-collapse: collapse; margin: 10px 0; font-size:.92rem; display:block; overflow-x:auto; }
	th,td { text-align:left; padding:10px 12px; border-bottom:1px solid var(--border-soft); vertical-align:top; }
	th { color: var(--text-dim); font-size:.83rem; }
	.muted { color: var(--text-dim); }
	@media (max-width: 900px) { .grid { grid-template-columns: 1fr; } }
</style>
