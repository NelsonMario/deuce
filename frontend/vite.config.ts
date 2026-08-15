import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';
import { defineConfig } from 'vite';
import pkg from './package.json' with { type: 'json' };

const { version } = pkg;

export default defineConfig({
	define: {
		// Baked in at build time so the footer/about screen can show which
		// build a bug report was filed against — see CHANGELOG.md.
		__APP_VERSION__: JSON.stringify(version)
	},
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter({
				pages: 'build',
				assets: 'build',
				fallback: 'index.html',
				precompress: false,
				strict: true
			})
		}),
		SvelteKitPWA({
			registerType: 'autoUpdate',
			injectRegister: false,
			strategies: 'generateSW',
			manifest: {
				name: 'deuce — court rotation',
				short_name: 'deuce',
				description: 'Fair player rotation and rating-balanced matches for badminton club sessions.',
				theme_color: '#0a0a0b',
				background_color: '#0a0a0b',
				display: 'standalone',
				orientation: 'portrait-primary',
				start_url: '/',
				scope: '/',
				icons: [
					{ src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
					{ src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
					{ src: '/icons/icon-maskable.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' }
				]
			},
			workbox: {
				navigateFallback: '/index.html',
				globPatterns: ['**/*.{js,css,html,svg,png,webmanifest}']
			},
			devOptions: {
				enabled: false
			}
		})
	]
});
