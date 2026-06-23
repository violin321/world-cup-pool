<script lang="ts">
	import { onMount } from 'svelte';
	import { pb } from '$lib/pb';

	type Summary = { users: number; leagues: number; tips: number; matches: { total: number; scheduled: number; live: number; finished: number } };
	type Match = { id: string; extId: string; stage: string; num: number; groupLetter: string; roundLabel: string; kickoff: string; status: string; homeTeam: string; awayTeam: string; ftHome: number; ftAway: number; etHome: number; etAway: number; penHome: number; penAway: number; finalizedAt: string };
	type Tip = { id: string; userName: string; userEmail: string; match: Match; ftHome: number; ftAway: number; etHome: number; etAway: number; updated: string };
	type UserRow = { id: string; name: string; email: string; tips: number; leagues: number; created: string };
	type LeagueRow = { id: string; name: string; inviteCode: string; ownerName: string; members: number; created: string };

	let checking = $state(true);
	let isAdmin = $state(false);
	let error = $state('');
	let email = $state('');
	let password = $state('');
	let tab = $state<'overview' | 'matches' | 'tips' | 'users'>('overview');
	let busy = $state(false);
	let summary = $state<Summary | null>(null);
	let matches = $state<Match[]>([]);
	let tips = $state<Tip[]>([]);
	let users = $state<UserRow[]>([]);
	let leagues = $state<LeagueRow[]>([]);
	let matchQuery = $state('');
	let tipUserQuery = $state('');
	let tipMatchQuery = $state('');

	const statusLabels: Record<string, string> = { scheduled: '未开始', live: '进行中', finished: '已结束', postponed: '延期', cancelled: '取消' };
	const stageLabels: Record<string, string> = { group: '小组赛', R32: '32 强', R16: '16 强', QF: '四分之一决赛', SF: '半决赛', '3RD': '季军赛', FINAL: '决赛' };

	const filteredMatches = $derived(matches.filter((m) => {
		const q = matchQuery.trim().toLowerCase();
		if (!q) return true;
		return `${m.homeTeam} ${m.awayTeam} ${m.stage} ${m.groupLetter} ${m.status}`.toLowerCase().includes(q);
	}));

	function fmtDate(v: string) {
		if (!v) return '—';
		return new Date(v).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' });
	}
	async function api<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
		return pb.send(path, { method, body });
	}
	async function verify() {
		checking = true; error = '';
		try {
			await api('/api/admin/me');
			isAdmin = true;
			await loadAll();
		} catch (err) {
			isAdmin = false;
			if (pb.authStore.isValid) error = '当前登录不是 PocketBase superuser，无法访问管理后台。';
		} finally { checking = false; }
	}
	async function loginSuperuser() {
		busy = true; error = '';
		try {
			await pb.collection('_superusers').authWithPassword(email.trim(), password);
			await verify();
		} catch (err) { error = '登录失败：请使用 PocketBase superuser 邮箱和密码。'; }
		finally { busy = false; }
	}
	function logout() { pb.authStore.clear(); isAdmin = false; summary = null; }
	async function loadAll() { await Promise.all([loadSummary(), loadMatches(), loadTips(), loadUsers(), loadLeagues()]); }
	async function loadSummary() { summary = await api('/api/admin/summary'); }
	async function loadMatches() { matches = (await api<{ items: Match[] }>('/api/admin/matches')).items; }
	async function loadTips() {
		const qs = new URLSearchParams();
		if (tipUserQuery.trim()) qs.set('user', tipUserQuery.trim());
		if (tipMatchQuery.trim()) qs.set('match', tipMatchQuery.trim());
		tips = (await api<{ items: Tip[] }>(`/api/admin/tips?${qs}`)).items;
	}
	async function loadUsers() { users = (await api<{ items: UserRow[] }>('/api/admin/users')).items; }
	async function loadLeagues() { leagues = (await api<{ items: LeagueRow[] }>('/api/admin/leagues')).items; }
	async function saveResult(m: Match) {
		const msg = `确认覆盖赛果？\n${m.homeTeam} vs ${m.awayTeam}\n新比分：${m.ftHome}-${m.ftAway}\n状态：${statusLabels[m.status] ?? m.status}`;
		if (!confirm(msg)) return;
		busy = true; error = '';
		try {
			await api(`/api/admin/matches/${m.id}/result`, 'POST', { status: m.status, ftHome: Number(m.ftHome), ftAway: Number(m.ftAway), etHome: Number(m.etHome || 0), etAway: Number(m.etAway || 0), penHome: Number(m.penHome || 0), penAway: Number(m.penAway || 0) });
			await Promise.all([loadMatches(), loadSummary()]);
		} catch (err) { error = '保存赛果失败，请检查输入。'; }
		finally { busy = false; }
	}
	async function recompute() {
		if (!confirm('确认重新计算所有用户积分？这会重建积分明细。')) return;
		busy = true; error = '';
		try { await api('/api/admin/recompute', 'POST', {}); await loadSummary(); alert('积分已重新计算。'); }
		catch (err) { error = '重新计算失败。'; }
		finally { busy = false; }
	}
	async function deleteTip(t: Tip) {
		const ok = prompt(`高风险操作：将清空该预测。\n用户：${t.userName}\n比赛：${t.match.homeTeam} vs ${t.match.awayTeam}\n请输入“删除”确认。`);
		if (ok !== '删除') return;
		busy = true; error = '';
		try { await api(`/api/admin/tips/${t.id}`, 'DELETE'); await Promise.all([loadTips(), loadSummary()]); }
		catch (err) { error = '删除预测失败。'; }
		finally { busy = false; }
	}
	onMount(() => { void verify(); });
</script>

<svelte:head><title>管理后台 · World Cup Pool</title></svelte:head>

<main class="admin-page">
	<header class="hero">
		<div>
			<p class="eyebrow">World Cup Pool</p>
			<h1>中文管理后台</h1>
			<p>日常运营入口：赛果录入、预测清空、积分重算、用户与联赛概览。</p>
		</div>
		{#if isAdmin}<button class="ghost" onclick={logout}>退出 superuser</button>{/if}
	</header>

	{#if checking}
		<section class="card">正在校验管理员身份…</section>
	{:else if !isAdmin}
		<section class="card login-card">
			<h2>需要 PocketBase superuser 权限</h2>
			<p>普通用户不会加载任何后台数据。请使用 PocketBase superuser 登录。</p>
			{#if error}<p class="error">{error}</p>{/if}
			<form onsubmit={(e) => { e.preventDefault(); void loginSuperuser(); }}>
				<label>邮箱 <input bind:value={email} autocomplete="username" /></label>
				<label>密码 <input bind:value={password} type="password" autocomplete="current-password" /></label>
				<button disabled={busy}>{busy ? '登录中…' : '登录管理后台'}</button>
			</form>
			<a href="/_/" target="_blank" rel="noreferrer">高级入口：PocketBase 原始后台</a>
		</section>
	{:else}
		{#if error}<p class="error banner">{error}</p>{/if}
		<nav class="tabs">
			<button class:active={tab === 'overview'} onclick={() => (tab = 'overview')}>概览</button>
			<button class:active={tab === 'matches'} onclick={() => (tab = 'matches')}>比赛结果</button>
			<button class:active={tab === 'tips'} onclick={() => (tab = 'tips')}>用户预测</button>
			<button class:active={tab === 'users'} onclick={() => (tab = 'users')}>用户与联赛</button>
		</nav>

		{#if tab === 'overview'}
			<section class="grid cards">
				<div class="card metric"><span>用户</span><strong>{summary?.users ?? '—'}</strong></div>
				<div class="card metric"><span>联赛</span><strong>{summary?.leagues ?? '—'}</strong></div>
				<div class="card metric"><span>预测</span><strong>{summary?.tips ?? '—'}</strong></div>
				<div class="card metric"><span>比赛</span><strong>{summary?.matches.total ?? '—'}</strong><small>未开始 {summary?.matches.scheduled ?? 0} · 进行中 {summary?.matches.live ?? 0} · 已结束 {summary?.matches.finished ?? 0}</small></div>
			</section>
			<section class="card actions"><h2>高风险操作</h2><button class="danger" disabled={busy} onclick={recompute}>重新计算所有积分</button><a href="/_/" target="_blank" rel="noreferrer">打开 PocketBase 高级后台</a></section>
		{:else if tab === 'matches'}
			<section class="card"><div class="toolbar"><h2>比赛结果</h2><input placeholder="搜索球队/阶段/状态" bind:value={matchQuery} /><button onclick={loadMatches}>刷新</button></div><div class="table-wrap"><table><thead><tr><th>时间</th><th>阶段</th><th>比赛</th><th>状态</th><th>90 分钟</th><th>加时</th><th>点球</th><th>操作</th></tr></thead><tbody>{#each filteredMatches as m (m.id)}<tr><td>{fmtDate(m.kickoff)}</td><td>{stageLabels[m.stage] ?? m.stage}</td><td><strong>{m.homeTeam}</strong> vs <strong>{m.awayTeam}</strong></td><td><select bind:value={m.status}><option value="scheduled">未开始</option><option value="live">进行中</option><option value="finished">已结束</option><option value="postponed">延期</option><option value="cancelled">取消</option></select></td><td><input class="score" type="number" min="0" max="99" bind:value={m.ftHome} /> - <input class="score" type="number" min="0" max="99" bind:value={m.ftAway} /></td><td><input class="score" type="number" min="0" max="99" bind:value={m.etHome} /> - <input class="score" type="number" min="0" max="99" bind:value={m.etAway} /></td><td><input class="score" type="number" min="0" max="99" bind:value={m.penHome} /> - <input class="score" type="number" min="0" max="99" bind:value={m.penAway} /></td><td><button disabled={busy} onclick={() => saveResult(m)}>保存</button></td></tr>{/each}</tbody></table></div></section>
		{:else if tab === 'tips'}
			<section class="card"><div class="toolbar"><h2>用户预测</h2><input placeholder="按用户名/邮箱" bind:value={tipUserQuery} /><input placeholder="按球队/比赛" bind:value={tipMatchQuery} /><button onclick={loadTips}>搜索</button></div><div class="table-wrap"><table><thead><tr><th>用户</th><th>比赛</th><th>预测</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{#each tips as t (t.id)}<tr><td><strong>{t.userName}</strong><br /><small>{t.userEmail}</small></td><td>{fmtDate(t.match.kickoff)}<br />{t.match.homeTeam} vs {t.match.awayTeam}</td><td>{t.ftHome} - {t.ftAway}</td><td>{fmtDate(t.updated)}</td><td><button class="danger" disabled={busy} onclick={() => deleteTip(t)}>清空预测</button></td></tr>{/each}</tbody></table></div></section>
		{:else}
			<section class="split"><div class="card"><h2>用户</h2><div class="table-wrap"><table><thead><tr><th>姓名</th><th>邮箱</th><th>预测</th><th>联赛</th></tr></thead><tbody>{#each users as u (u.id)}<tr><td>{u.name}</td><td>{u.email}</td><td>{u.tips}</td><td>{u.leagues}</td></tr>{/each}</tbody></table></div></div><div class="card"><h2>联赛</h2><div class="table-wrap"><table><thead><tr><th>名称</th><th>邀请码</th><th>所有者</th><th>成员</th></tr></thead><tbody>{#each leagues as l (l.id)}<tr><td>{l.name}</td><td>{l.inviteCode}</td><td>{l.ownerName}</td><td>{l.members}</td></tr>{/each}</tbody></table></div></div></section>
		{/if}
	{/if}
</main>

<style>
	.admin-page{width:min(1180px,100%);margin:0 auto;padding:2rem 1rem 4rem;color:var(--text)}.hero{display:flex;justify-content:space-between;gap:1rem;align-items:flex-start;margin-bottom:1.25rem}.eyebrow{letter-spacing:.12em;text-transform:uppercase;color:var(--muted);font-weight:700}.hero h1{font-size:clamp(2rem,5vw,4rem);margin:.2rem 0}.card{background:var(--panel);border:1px solid var(--border);border-radius:22px;padding:1.1rem;box-shadow:var(--shadow)}.grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:1rem}.metric span{color:var(--muted)}.metric strong{display:block;font-size:2.2rem;margin:.35rem 0}.metric small{color:var(--muted)}.tabs{display:flex;gap:.5rem;flex-wrap:wrap;margin:1rem 0}.tabs button,.card button,.ghost{border:1px solid var(--border);border-radius:999px;padding:.65rem 1rem;background:var(--panel-strong);color:var(--text);font-weight:700;cursor:pointer}.tabs button.active,.card button:not(.danger){background:var(--accent);color:white}.danger{background:#fee2e2!important;color:#991b1b!important;border-color:#fecaca!important}.ghost{background:transparent}.login-card{max-width:520px}.login-card form{display:grid;gap:.8rem;margin:1rem 0}.login-card input,.toolbar input,select,.score{border:1px solid var(--border);border-radius:12px;padding:.55rem;background:var(--bg);color:var(--text)}label{display:grid;gap:.35rem}.error{color:#b91c1c;font-weight:700}.banner{margin:.5rem 0}.toolbar{display:flex;gap:.75rem;align-items:center;flex-wrap:wrap;margin-bottom:1rem}.toolbar h2{margin-right:auto}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse;min-width:760px}th,td{padding:.7rem;border-bottom:1px solid var(--border);text-align:left;vertical-align:middle}th{color:var(--muted);font-size:.85rem}.score{width:4.5rem}.actions{margin-top:1rem;display:flex;gap:1rem;align-items:center;flex-wrap:wrap}.split{display:grid;grid-template-columns:1fr 1fr;gap:1rem}@media (max-width:800px){.grid,.split{grid-template-columns:1fr}.hero{display:block}table{min-width:900px}}
</style>
