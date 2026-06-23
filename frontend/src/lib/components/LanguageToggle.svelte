<script lang="ts">
	import { Languages } from '@lucide/svelte';
	import { language, type LanguageCode } from '$lib/language.svelte';

	let { compact = false }: { compact?: boolean } = $props();

	const languageOptions: { code: LanguageCode; label: string }[] = [
		{ code: 'en', label: 'English' },
		{ code: 'zh-CN', label: '简体中文' },
		{ code: 'nb', label: 'Bokmål' },
		{ code: 'nn', label: 'Nynorsk' }
	];

	function onLanguageChange(event: Event) {
		const next = (event.currentTarget as HTMLSelectElement).value as LanguageCode;
		language.set(next);
	}
</script>

<label class="language-select" class:compact title="Language / 语言">
	<Languages size={18} aria-hidden="true" />
	<span class="sr-only">Language / 语言</span>
	<select aria-label="Language / 语言" value={language.resolved} onchange={onLanguageChange}>
		{#each languageOptions as option}
			<option value={option.code}>{option.label}</option>
		{/each}
	</select>
</label>

<style>
	.language-select {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		width: auto;
		height: 38px;
		padding: 0 0.55rem 0 0.7rem;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius-pill);
		color: var(--text);
		font-weight: 600;
		font-size: 0.85rem;
		transition:
			background 0.15s ease,
			border-color 0.15s ease,
			color 0.15s ease;
	}
	.language-select.compact {
		width: auto;
		min-width: 92px;
		padding: 0 0.35rem 0 0.55rem;
	}
	.language-select:hover,
	.language-select:focus-within {
		border-color: var(--border-strong);
		background: var(--surface-3);
		color: var(--accent);
	}
	.language-select:focus-within {
		outline: var(--ring);
		outline-offset: 2px;
	}
	.language-select select {
		max-width: 8.5rem;
		border: none;
		background: transparent;
		color: inherit;
		font: inherit;
		font-weight: 700;
		cursor: pointer;
		outline: none;
	}
	.language-select.compact select {
		max-width: 5.3rem;
	}
	.language-select select option {
		color: var(--text);
		background: var(--surface-2);
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
