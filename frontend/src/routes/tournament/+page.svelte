<script lang="ts">
	import { browser } from '$app/environment';
	import { tipsStore, type Match, type Team } from '$lib/tips.svelte';
	import Flag from '$lib/components/Flag.svelte';
	import { collapseOnScroll } from '$lib/actions';
	import { serverClock } from '$lib/serverclock.svelte';
	import { teamDisplayName, teamNameFromFifaCode } from '$lib/teamNames';
	import { LocateFixed } from '@lucide/svelte';
	import { language } from '$lib/language.svelte';
	import { stageName as knockoutStageName } from '$lib/stageLabels';
	import { forecastStore as fs } from '$lib/forecast.svelte';

	let view = $state<'groups' | 'bracket' | 'topscorer'>('groups');
	let bracketLayout = $state<'list' | 'classic'>('classic');

	$effect(() => {
		if (!tipsStore.loaded) tipsStore.load().catch(() => {});
		if (!fs.loaded) fs.load().catch(() => {});
	});

	$effect(() => {
		if (!browser) return;
		const saved = localStorage.getItem('tournamentBracketLayout');
		if (saved === 'list' || saved === 'classic') bracketLayout = saved;
	});

	$effect(() => {
		if (!browser) return;
		localStorage.setItem('tournamentBracketLayout', bracketLayout);
	});

	function played(m: Match) {
		return m.status === 'finished' || !!m.finalizedAt;
	}

	interface Standing {
		id: string;
		p: number;
		w: number;
		d: number;
		l: number;
		gf: number;
		ga: number;
		pts: number;
	}

	// Live group tables from finished group matches.
	let groups = $derived.by(() => {
		const blank = (id: string): Standing => ({
			id,
			p: 0,
			w: 0,
			d: 0,
			l: 0,
			gf: 0,
			ga: 0,
			pts: 0
		});
		const byG: Record<string, Record<string, Standing>> = {};
		// Seed every group with all its teams so the table is always full.
		for (const [letter, ids] of Object.entries(
			tipsStore.tournamentGroups
		)) {
			byG[letter] = {};
			for (const id of ids) byG[letter][id] = blank(id);
		}
		for (const m of tipsStore.matches) {
			if (m.stage !== 'group' || !played(m)) continue;
			const g = m.groupLetter;
			(byG[g] ||= {});
			for (const id of [m.homeTeam, m.awayTeam])
				byG[g][id] ||= blank(id);
			const H = byG[g][m.homeTeam];
			const A = byG[g][m.awayTeam];
			H.p++;
			A.p++;
			H.gf += m.ftHome;
			H.ga += m.ftAway;
			A.gf += m.ftAway;
			A.ga += m.ftHome;
			if (m.ftHome > m.ftAway) {
				H.w++;
				A.l++;
				H.pts += 3;
			} else if (m.ftHome < m.ftAway) {
				A.w++;
				H.l++;
				A.pts += 3;
			} else {
				H.d++;
				A.d++;
				H.pts++;
				A.pts++;
			}
		}
		return Object.entries(byG)
			.map(([letter, tbl]) => ({
				letter,
				rows: Object.values(tbl).sort(
					(a, b) =>
						b.pts - a.pts ||
						b.gf - b.ga - (a.gf - a.ga) ||
						b.gf - a.gf
				)
			}))
			.sort((a, b) => a.letter.localeCompare(b.letter));
	});

	const stages = ['R32', 'R16', 'QF', 'SF', '3RD', 'FINAL'];
	let bracket = $derived(
		stages.map((s) => ({
			stage: s,
			matches: tipsStore.matches
				.filter((m) => m.stage === s)
				.sort((a, b) => a.num - b.num)
		}))
	);

	const classicStages = ['R32', 'R16', 'QF', 'SF', 'FINAL'];
	let classicBracket = $derived(
		classicStages.map((s) => ({
			stage: s,
			matches: tipsStore.matches
				.filter((m) => m.stage === s)
				.sort((a, b) => a.num - b.num)
		}))
	);
	let thirdPlaceMatches = $derived(
		tipsStore.matches
			.filter((m) => m.stage === '3RD')
			.sort((a, b) => a.num - b.num)
	);

	// Current knockout stage = stage of the next KO match not yet started
	// (or the last stage once it's all done).
	let currentStage = $derived.by(() => {
		const now = serverClock.now();
		const ko = tipsStore.matches
			.filter((m) => m.stage !== 'group')
			.sort(
				(a, b) =>
					new Date(a.kickoff).getTime() -
					new Date(b.kickoff).getTime()
			);
		const next = ko.find((m) => new Date(m.kickoff).getTime() >= now);
		return next?.stage ?? ko[ko.length - 1]?.stage ?? '';
	});

	function goNow() {
		document
			.getElementById(`st-${currentStage}`)
			?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	function tn(id: string) {
		return tipsStore.team(id);
	}
	function tname(id: string, fallback = '?') {
		return teamDisplayName(tn(id), fallback);
	}
	function scoreText(m: Match) {
		if (!played(m)) return '';
		let s = `${m.ftHome}–${m.ftAway}`;
		if (m.etHome || m.etAway) s = `${m.etHome}–${m.etAway} ${language.text('e.e.o.', 'e.eo.', 'aet')}`;
		if (m.penHome || m.penAway) s += ` (${m.penHome}–${m.penAway} ${language.text('str', 'str', 'pens')})`;
		return s;
	}

	function stageShort(stage: string) {
		const labels: Record<string, string> = {
			R32: language.text('32', '32', 'R32'),
			R16: language.text('16', '16', 'R16'),
			QF: language.text('Kvart', 'Kvart', 'QF'),
			SF: language.text('Semi', 'Semi', 'SF'),
			FINAL: language.text('Finale', 'Finale', 'Final')
		};
		if (language.isChinese) return ({ R32: '32强', R16: '16强', QF: '1/4', SF: '半决', FINAL: '决赛' } as Record<string, string>)[stage] ?? stage;
		return labels[stage] ?? stage;
	}

	function kickoffText(iso: string) {
		const date = new Date(iso);
		if (!Number.isFinite(date.getTime())) return '';
		return date.toLocaleString(language.locale, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function teamCode(team: Team | null | undefined, fallback: string) {
		return team?.fifaCode || fallback?.match(/[A-Z0-9]{2,4}/)?.[0] || 'TBD';
	}

	function teamSource(team: Team | null | undefined, fallback: string) {
		if (team) return team.fifaCode;
		return fallback || language.text('Ikke klar', 'Ikkje klar', 'TBD');
	}

	function teamSeedLabel(team: Team | null | undefined, fallback: string) {
		if (team) return teamDisplayName(team, fallback);
		return fallback || language.text('Ikke klar', 'Ikkje klar', 'TBD');
	}

	function initials(name: string) {
		const parts = name.split(/\s+/).filter(Boolean);
		if (parts.length <= 1) return [...name].slice(0, 2).join('').toUpperCase();
		return parts
			.slice(0, 2)
			.map((part) => part[0]?.toUpperCase() ?? '')
			.join('');
	}

	function gbTeamName(player: { teamId: string; teamName: string }) {
		const team = fs.team(player.teamId);
		return teamNameFromFifaCode(team?.fifaCode, player.teamName);
	}

	function updatedAt(iso?: string) {
		if (!iso) return language.text('Ikke synket ennå', 'Ikkje synka enno', 'Not synced yet');
		const date = new Date(iso);
		if (!Number.isFinite(date.getTime())) return '';
		const day = String(date.getDate()).padStart(2, '0');
		const month = String(date.getMonth() + 1).padStart(2, '0');
		const hour = String(date.getHours()).padStart(2, '0');
		const minute = String(date.getMinutes()).padStart(2, '0');
		return `${day}.${month} ${hour}:${minute}`;
	}
</script>

<div class="stickyhead" use:collapseOnScroll>
	<p class="kicker">VM 2026</p>
	<div class="sh-expand"><div class="sh-inner"><h1>{language.text('Turnering', 'Turnering', 'Tournament')}</h1></div></div>
	<div class="seg">
		<button class:on={view === 'groups'} onclick={() => (view = 'groups')}>{language.text('Gruppetabeller', 'Gruppetabellar', 'Group tables')}</button>
		<button class:on={view === 'bracket'} onclick={() => (view = 'bracket')}>{language.text('Sluttspill', 'Sluttspel', 'Knockout bracket')}</button>
		<button class:on={view === 'topscorer'} onclick={() => (view = 'topscorer')}>{language.text('Toppscorere', 'Toppscorarar', 'Top scorers')}</button>
	</div>
</div>

{#if !tipsStore.loaded}
	<p class="muted">{language.text('Laster...', 'Lastar…', 'Loading…')}</p>
{:else if view === 'groups'}
	{#if groups.length === 0}
		<div class="card empty">
			<p class="muted">
				{language.text(
					'Ingen gruppespillkamper er spilt ennå. Tabellene fylles når resultatene kommer.',
					'Ingen gruppespelkampar er spelte enno. Tabellane blir fylte når resultata kjem.',
					'No group-stage matches have been played yet. The tables will fill as results come in.'
				)}
			</p>
		</div>
	{:else}
		<div class="gwrap stagger">
			{#each groups as g (g.letter)}
				<section class="card grp">
					<div class="ghead"><span class="gl">{g.letter}</span> {language.text('Gruppe', 'Gruppe', 'Group')} {g.letter}</div>
					<table>
						<thead>
							<tr><th></th><th>{language.text('Lag', 'Lag', 'Team')}</th><th>{language.text('K', 'K', 'P')}</th><th>{language.text('MF', 'MF', 'GD')}</th><th>{language.text('P', 'P', 'Pts')}</th></tr>
						</thead>
						<tbody>
							{#each g.rows as r, i (r.id)}
								<tr class:adv={i < 2} class:third={i === 2}>
									<td class="rk">{i + 1}</td>
									<td class="tm">
										<Flag iso2={tn(r.id)?.iso2 ?? ''} code={tn(r.id)?.fifaCode ?? ''} />
										<span>{tname(r.id)}</span>
									</td>
									<td class="digits">{r.p}</td>
									<td class="digits">{r.gf - r.ga > 0 ? '+' : ''}{r.gf - r.ga}</td>
									<td class="digits pts">{r.pts}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</section>
			{/each}
		</div>
	{/if}
{:else if view === 'bracket'}
	<section class="ko-toolbar card" aria-label={language.text('Sluttspillvisning', 'Sluttspelvising', 'Knockout view')}>
		<div>
			<h2>{language.text('Sluttspill', 'Sluttspel', 'Knockout bracket')}</h2>
			<p class="muted">
				{language.text(
					'Bytt mellom kompakt kampliste og klassisk turneringstre med flagg og runde-kolonner.',
					'Byt mellom kompakt kampliste og klassisk turneringstre med flagg og runde-kolonnar.',
					'Switch between the compact match list and a classic tournament tree with flags and round columns.'
				)}
			</p>
		</div>
		<div class="seg ko-seg">
			<button class:on={bracketLayout === 'list'} onclick={() => (bracketLayout = 'list')}>
				{language.text('Kompakt liste', 'Kompakt liste', 'Compact list')}
			</button>
			<button class:on={bracketLayout === 'classic'} onclick={() => (bracketLayout = 'classic')}>
				{language.text('Klassisk tre', 'Klassisk tre', 'Classic bracket')}
			</button>
		</div>
	</section>

	{#if bracketLayout === 'list'}
		<div class="stagger">
			{#each bracket as col (col.stage)}
				<h3 class="rname" id={`st-${col.stage}`}>{knockoutStageName(col.stage)}</h3>
				{#each col.matches as m (m.id)}
					{@const H = tn(m.homeTeam)}
					{@const A = tn(m.awayTeam)}
					{@const done = played(m)}
					<div class="bm card">
						<div class="side" class:won={done && m.advancer === m.homeTeam}>
							{#if H}<Flag iso2={H.iso2} code={H.fifaCode} />{/if}
							<span class="nm" class:ph={!H}>{teamDisplayName(H, m.homeLabel)}</span>
						</div>
						<div class="mid digits">
							{#if done}{scoreText(m)}{:else}<span class="vs">vs</span>{/if}
							{#if tipsStore.liveMatchIds.has(m.id)}
								<span class="live-badge">LIVE</span>
							{/if}
						</div>
						<div class="side right" class:won={done && m.advancer === m.awayTeam}>
							<span class="nm" class:ph={!A}>{teamDisplayName(A, m.awayLabel)}</span>
							{#if A}<Flag iso2={A.iso2} code={A.fifaCode} />{/if}
						</div>
					</div>
				{/each}
			{/each}
			<div class="fabpad"></div>
		</div>
	{:else}
		<div class="classic-hint muted">
			{language.text('Sveip sidelengs for å lese hele treet.', 'Sveip sidelengs for å lese heile treet.', 'Swipe sideways to read the full bracket.')}
		</div>
		<div class="classic-shell card">
			<div class="classic-bracket" style={`--rounds: ${classicBracket.length}`}>
				{#each classicBracket as col (col.stage)}
					<section class={`classic-round stage-${col.stage.toLowerCase()}`} id={`st-${col.stage}`}>
						<header class="round-head">
							<span class="round-kicker">{stageShort(col.stage)}</span>
							<h3>{knockoutStageName(col.stage)}</h3>
						</header>
						<div class="round-stack">
							{#each col.matches as m (m.id)}
								{@const H = tn(m.homeTeam)}
								{@const A = tn(m.awayTeam)}
								{@const done = played(m)}
								<article class="tree-match" class:done class:live={tipsStore.liveMatchIds.has(m.id)}>
									<div class="match-meta">
										<span>#{m.num}</span>
										<span>{kickoffText(m.kickoff)}</span>
									</div>
									<div class="tree-team" class:won={done && m.advancer === m.homeTeam} class:tbd={!H}>
										<span class="flag-slot">{#if H}<Flag iso2={H.iso2} code={H.fifaCode} />{:else}<span>{teamCode(H, m.homeLabel)}</span>{/if}</span>
										<span class="team-main">
											<span class="team-name">{teamSeedLabel(H, m.homeLabel)}</span>
											<span class="team-code">{teamSource(H, m.homeLabel)}</span>
										</span>
										<span class="team-score digits">{#if done}{m.ftHome}{:else}–{/if}</span>
									</div>
									<div class="tree-team" class:won={done && m.advancer === m.awayTeam} class:tbd={!A}>
										<span class="flag-slot">{#if A}<Flag iso2={A.iso2} code={A.fifaCode} />{:else}<span>{teamCode(A, m.awayLabel)}</span>{/if}</span>
										<span class="team-main">
											<span class="team-name">{teamSeedLabel(A, m.awayLabel)}</span>
											<span class="team-code">{teamSource(A, m.awayLabel)}</span>
										</span>
										<span class="team-score digits">{#if done}{m.ftAway}{:else}–{/if}</span>
									</div>
									{#if done && (m.penHome || m.penAway)}
										<p class="match-note">{scoreText(m)}</p>
									{/if}
								</article>
							{/each}
						</div>
					</section>
				{/each}
			</div>
		</div>
		{#if thirdPlaceMatches.length}
			<section class="third-strip card">
				<h3>{knockoutStageName('3RD')}</h3>
				{#each thirdPlaceMatches as m (m.id)}
					{@const H = tn(m.homeTeam)}
					{@const A = tn(m.awayTeam)}
					{@const done = played(m)}
					<div class="mini-third">
						<span class:won={done && m.advancer === m.homeTeam}>{teamSeedLabel(H, m.homeLabel)}</span>
						<b>{#if done}{scoreText(m)}{:else}vs{/if}</b>
						<span class:won={done && m.advancer === m.awayTeam}>{teamSeedLabel(A, m.awayLabel)}</span>
					</div>
				{/each}
			</section>
		{/if}
	{/if}
{:else if view === 'topscorer'}
	<section class="card gb-live">
		<div class="gb-live-head">
			<p class="muted small">
				{language.text('Den offisielle toppscorertabellen.', 'Den offisielle toppscorartabellen.', 'The official Golden Boot standings.')}
				{#if fs.goldenBoot.source}
					<span class="source-pill">{language.text('Kilde', 'Kjelde', 'Source')}: {fs.goldenBoot.source}</span>
				{/if}
			</p>
			{#if fs.goldenBoot.updatedAt}
				<p class="muted gb-updated">{language.text('Sist', 'Sist', 'Updated')} {updatedAt(fs.goldenBoot.updatedAt)}</p>
			{/if}
		</div>
		<table class="gb-table">
			<thead>
				<tr>
					<th>#</th>
					<th>{language.text('Spiller', 'Spelar', 'Player')}</th>
					<th>{language.text('Lag', 'Lag', 'Team')}</th>
					<th class="num">{language.text('Mål', 'Mål', 'Goals')}</th>
				</tr>
			</thead>
			<tbody>
				{#if fs.goldenBoot.leaders.length === 0}
					<tr>
						<td colspan="4" style="text-align: center; color: var(--muted); padding: 1.5rem 0;">
							{language.text('Ingen mål registrert ennå.', 'Ingen mål registrert enno.', 'No goals registered yet.')}
						</td>
					</tr>
				{/if}
				{#each fs.goldenBoot.leaders as player (player.id)}
					<tr class:picked={fs.goldenBootPlayer === player.id}>
						<td>{player.rank || '–'}</td>
						<td>
							<span class="gb-row-player">
								{#if player.photoUrl}<img class="mini-headshot" src={player.photoUrl} alt="" loading="lazy" />{:else}<span class="mini-headshot fallback">{initials(player.name)}</span>{/if}
								<b>{player.name}</b>
							</span>
						</td>
						<td>
							<span class="gb-team">
								<Flag iso2={fs.team(player.teamId)?.iso2 ?? ''} code={fs.team(player.teamId)?.fifaCode ?? ''} />
								<span class="tm-full">{gbTeamName(player)}</span>
								<span class="tm-short">{gbTeamName(player)}</span>
							</span>
						</td>
						<td class="num digits">{player.goals}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</section>
{/if}

{#if tipsStore.loaded && view === 'bracket' && currentStage}
	<button class="fab" onclick={goNow} aria-label={language.text('Hopp til aktuell runde', 'Hopp til aktuell runde', 'Jump to current round')}>
		<LocateFixed size={18} /> {language.text('Nå', 'No', 'Now')}
	</button>
{/if}

<style>
	.stickyhead {
		position: sticky;
		top: var(--topbar-h);
		z-index: 20;
		margin: 0 -1rem;
		padding: 0.6rem 1rem 0.75rem;
		background: var(--bg);
		border-bottom: 1px solid var(--border);
	}
	.stickyhead h1 {
		margin: 0.1rem 0 0.7rem;
	}
	.stickyhead .seg {
		margin: 0;
	}
	@media (min-width: 900px) {
		.stickyhead {
			top: 0;
			margin: 0 -2rem;
			padding: 0.75rem 2rem 0.85rem;
		}
	}

	/* Top scorers */
	.gb-live-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 0.6rem;
	}
	.gb-live-head .small {
		margin: 0;
	}
	.source-pill {
		display: inline-flex;
		align-items: center;
		margin-left: 0.4rem;
		padding: 0.12rem 0.42rem;
		border-radius: 999px;
		border: 1px solid var(--border);
		background: var(--surface-2);
		color: var(--muted);
		font-size: 0.68rem;
		font-weight: 700;
	}
	.gb-updated {
		text-align: right;
		white-space: nowrap;
		font-size: 0.68rem;
		font-weight: 650;
		letter-spacing: 0.02em;
	}
	.gb-table {
		width: 100%;
		border-collapse: collapse;
	}
	.gb-table th,
	.gb-table td {
		padding: 0.55rem 0.35rem;
		border-bottom: 1px solid var(--border);
		text-align: left;
	}
	.gb-table th {
		color: var(--muted);
		font-size: 0.78rem;
		font-weight: 700;
		text-transform: none;
		letter-spacing: normal;
	}
	.gb-table tr.picked td {
		background: color-mix(in srgb, var(--accent) 8%, transparent);
	}
	.gb-row-player,
	.gb-team {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		min-width: 0;
	}
	.gb-row-player b,
	.gb-team {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.mini-headshot {
		display: inline-grid;
		place-items: center;
		border-radius: 50%;
		background: var(--surface);
		border: 1px solid var(--border);
		object-fit: cover;
		flex: none;
		width: 28px;
		height: 28px;
		font-size: 0.65rem;
	}
	.fallback {
		font-family: var(--font-display);
		font-weight: 800;
		color: var(--muted);
	}
	.tm-short { display: none; }
	@media (max-width: 500px) {
		.tm-full { display: none; }
		.tm-short { display: inline; }
	}

	.ko-toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
		padding: 1rem;
		min-width: 0;
		overflow: hidden;
	}
	.ko-toolbar > div:first-child {
		flex: 1 1 auto;
		min-width: 0;
	}
	.ko-toolbar h2 {
		font-size: 1.25rem;
		margin-bottom: 0.2rem;
	}
	.ko-toolbar p {
		margin: 0;
		max-width: 44rem;
		font-size: 0.86rem;
		line-height: 1.45;
	}
	.ko-seg {
		flex: 0 0 auto;
		width: auto;
		max-width: min(100%, 24rem);
		margin: 0;
		overflow-x: visible;
	}
	.ko-seg button {
		flex: 0 0 auto;
	}
	.classic-hint {
		margin: 0 0 0.55rem;
		font-size: 0.78rem;
	}
	.classic-shell {
		position: relative;
		max-width: 100%;
		padding: 1rem;
		overflow-x: auto;
		overflow-y: hidden;
		scrollbar-color: color-mix(in srgb, var(--accent) 35%, transparent) transparent;
		-webkit-overflow-scrolling: touch;
		overscroll-behavior-x: contain;
	}
	.classic-bracket {
		display: grid;
		grid-template-columns: repeat(var(--rounds), minmax(13.5rem, 1fr));
		gap: 1.25rem;
		width: max-content;
		min-width: 72rem;
		align-items: stretch;
	}
	.classic-round {
		display: grid;
		grid-template-rows: auto 1fr;
		gap: 0.75rem;
		min-width: 0;
	}
	.round-head {
		padding: 0.65rem 0.7rem;
		border: 1px solid var(--border);
		border-radius: 16px;
		background: color-mix(in srgb, var(--surface-2) 88%, transparent);
	}
	.round-head h3 {
		margin-top: 0.15rem;
		font-size: 0.98rem;
	}
	.round-kicker {
		display: inline-flex;
		align-items: center;
		padding: 0.08rem 0.45rem;
		border-radius: var(--radius-pill);
		background: color-mix(in srgb, var(--accent) 13%, transparent);
		color: var(--accent);
		font: 800 0.62rem var(--font);
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}
	.round-stack {
		display: flex;
		flex-direction: column;
		justify-content: space-around;
		gap: 0.85rem;
		min-height: 100%;
	}
	.tree-match {
		position: relative;
		border: 1px solid color-mix(in srgb, var(--border) 78%, transparent);
		border-radius: 18px;
		background:
			radial-gradient(circle at 0% 0%, color-mix(in srgb, var(--accent) 8%, transparent), transparent 42%),
			linear-gradient(180deg, color-mix(in srgb, var(--surface-2) 86%, transparent), var(--surface));
		box-shadow: 0 14px 34px -28px rgba(0, 0, 0, 0.95);
		overflow: visible;
	}
	.tree-match::after {
		content: '';
		position: absolute;
		left: calc(100% + 1px);
		top: 50%;
		width: 1.25rem;
		border-top: 1px solid color-mix(in srgb, var(--accent) 24%, var(--border));
	}
	.stage-final .tree-match::after {
		display: none;
	}
	.tree-match.live {
		border-color: #ff3b30;
		box-shadow: 0 0 0 1px rgba(255, 59, 48, 0.18), var(--glow);
	}
	.match-meta {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.42rem 0.65rem;
		border-bottom: 1px solid var(--border);
		color: var(--muted);
		font-size: 0.66rem;
		font-weight: 700;
	}
	.tree-team {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.55rem;
		padding: 0.62rem 0.65rem;
		border-bottom: 1px solid color-mix(in srgb, var(--border) 64%, transparent);
	}
	.tree-team:last-of-type {
		border-bottom: none;
	}
	.tree-team.won {
		background: linear-gradient(90deg, color-mix(in srgb, var(--accent) 10%, transparent), transparent);
	}
	.tree-team.tbd {
		color: var(--muted);
	}
	.flag-slot {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		border-radius: 999px;
		border: 1px solid var(--border);
		background: color-mix(in srgb, var(--surface-3) 70%, transparent);
		font-size: 0.62rem;
		font-weight: 900;
		letter-spacing: 0.02em;
	}
	.team-main {
		display: grid;
		min-width: 0;
		gap: 0.08rem;
	}
	.team-name,
	.team-code {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.team-name {
		font-weight: 800;
	}
	.team-code {
		color: var(--muted);
		font-size: 0.66rem;
		font-weight: 700;
		letter-spacing: 0.04em;
	}
	.team-score {
		min-width: 1.55rem;
		text-align: right;
		color: var(--accent);
		font-size: 1rem;
		font-weight: 900;
	}
	.match-note {
		margin: 0;
		padding: 0.35rem 0.65rem 0.5rem;
		border-top: 1px solid var(--border);
		color: var(--muted);
		font-size: 0.68rem;
		text-align: center;
	}
	.third-strip {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
		padding: 0.85rem 1rem;
	}
	.third-strip h3 {
		flex: none;
		font-size: 0.95rem;
	}
	.mini-third {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		min-width: 0;
		color: var(--muted);
	}
	.mini-third span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 14rem;
	}
	.mini-third .won {
		color: var(--accent);
		font-weight: 800;
	}
	@media (max-width: 760px) {
		.ko-toolbar {
			align-items: stretch;
			flex-direction: column;
			overflow: visible;
		}
		.ko-seg {
			width: 100%;
			max-width: none;
			overflow-x: auto;
		}
		.ko-seg button {
			flex: 1 1 0;
		}
		.classic-shell {
			margin: 0 -1rem;
			border-left: 0;
			border-right: 0;
			border-radius: 0;
		}
		.classic-bracket {
			grid-template-columns: repeat(var(--rounds), 15rem);
			min-width: 80rem;
		}
		.third-strip {
			align-items: flex-start;
			flex-direction: column;
		}
		.mini-third {
			width: 100%;
			justify-content: space-between;
		}
	}
	@media (max-width: 430px) {
		.ko-seg {
			flex-direction: column;
		}
		.ko-seg button {
			width: 100%;
		}
	}

	.gwrap {
		display: grid;
		gap: 0.85rem;
	}
	@media (min-width: 760px) {
		.gwrap {
			grid-template-columns: 1fr 1fr;
		}
	}
	.ghead {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-size: 0.85rem;
		margin-bottom: 0.6rem;
	}
	.gl {
		display: grid;
		place-items: center;
		width: 26px;
		height: 26px;
		border-radius: 7px;
		background: var(--surface-2);
		color: var(--text);
		border: 1px solid var(--border);
		font-family: var(--font-display);
		font-size: 0.95rem;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th {
		text-align: right;
		font-size: 0.66rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--muted);
		padding: 0 0.4rem 0.4rem;
	}
	th:nth-child(2) {
		text-align: left;
	}
	td {
		padding: 0.45rem 0.4rem;
		border-top: 1px solid var(--border);
		text-align: right;
	}
	.rk {
		width: 1.5rem;
		color: var(--muted);
		text-align: center;
	}
	.tm {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		text-align: left;
		font-weight: 600;
	}
	.pts {
		color: var(--accent);
		font-weight: 700;
	}
	tr.adv .rk {
		color: var(--accent);
		font-weight: 800;
	}
	tr.adv td {
		background: color-mix(in srgb, var(--accent) 7%, transparent);
	}
	tr.third .rk {
		color: var(--warning);
	}
	.rname {
		font-family: var(--font-display);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--muted);
		margin: 1.4rem 0 0.6rem;
		scroll-margin-top: 150px;
	}
	@media (min-width: 900px) {
		.rname {
			scroll-margin-top: 96px;
		}
	}
	.fabpad {
		height: 4rem;
	}
	.fab {
		position: fixed;
		right: 1rem;
		bottom: calc(var(--nav-h) + 1rem);
		z-index: 40;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.7rem 1rem;
		border: 1px solid var(--text);
		border-radius: var(--radius-pill);
		background: var(--text);
		color: var(--bg);
		font:
			800 0.8rem var(--font);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		cursor: pointer;
		box-shadow: var(--shadow-pop);
		transition:
			transform 0.12s ease,
			box-shadow 0.2s ease;
	}
	.fab:hover {
		transform: translateY(-2px);
		box-shadow: var(--glow);
	}
	@media (min-width: 900px) {
		.fab {
			bottom: 1.5rem;
			right: 1.5rem;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.fab {
			transition: none;
		}
	}
	.bm {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.7rem 0.9rem;
	}
	.bm + .bm {
		margin-top: 0.5rem;
	}
	.side {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
	}
	.side.right {
		justify-content: flex-end;
	}
	.nm {
		font-weight: 700;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.nm.ph {
		color: var(--muted);
		font-weight: 500;
	}
	.side.won .nm {
		color: var(--accent);
	}
	.mid {
		min-width: 4.5rem;
		text-align: center;
		font-size: 0.95rem;
		color: var(--text);
	}
	.vs {
		color: var(--muted);
		font-family: var(--font);
		font-size: 0.8rem;
	}
	.live-badge {
		display: block;
		margin: 0.1rem auto 0;
		padding: 0.1rem 0.35rem;
		border-radius: 4px;
		background: #ff3b30;
		color: white;
		font: 700 0.55rem var(--font);
		letter-spacing: 0.06em;
		text-transform: uppercase;
		animation: livePulse 1.5s ease-in-out infinite;
		width: fit-content;
	}
	@keyframes livePulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.45; }
	}
	.empty {
		text-align: center;
		padding: 2.5rem 1rem;
	}
	:global(:root[data-theme='worldcup']) .grp,
	:global(:root[data-theme='worldcup']) .bm,
	:global(:root[data-theme='worldcup']) .empty {
		background:
			radial-gradient(circle at 14% 0%, rgba(143, 197, 143, 0.075), transparent 32%),
			linear-gradient(180deg, rgba(13, 34, 40, 0.96), rgba(7, 17, 25, 0.98)),
			var(--surface);
		border-color: color-mix(in srgb, var(--accent) 12%, var(--border));
		box-shadow: 0 16px 42px -34px rgba(0, 0, 0, 0.9), inset 0 1px 0 rgba(255, 255, 255, 0.035);
	}
	:global(:root[data-theme='worldcup']) .grp::before,
	:global(:root[data-theme='worldcup']) .bm::before,
	:global(:root[data-theme='worldcup']) .empty::before {
		display: none;
	}
	:global(:root[data-theme='worldcup']) .gl {
		background: color-mix(in srgb, var(--surface-2) 78%, transparent);
		border-color: color-mix(in srgb, var(--accent) 12%, var(--border));
	}
	:global(:root[data-theme='worldcup']) td {
		border-top-color: color-mix(in srgb, var(--accent) 11%, var(--border));
	}
	:global(:root[data-theme='worldcup']) tr.adv td {
		background: color-mix(in srgb, var(--accent) 5%, transparent);
	}
	:global(:root[data-theme='worldcup']) .side.won .nm,
	:global(:root[data-theme='worldcup']) tr.adv .rk,
	:global(:root[data-theme='worldcup']) .pts {
		color: var(--accent);
	}
</style>
