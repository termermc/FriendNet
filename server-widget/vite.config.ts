import { defineConfig } from 'vite'

export default defineConfig({
	build: {
		lib: {
			entry: 'src/main.ts',
			name: 'FriendNetServerWidget',
			fileName: 'friendnet-server-widget',
			formats: ['es'],
		},

		rollupOptions: {
			output: {
				inlineDynamicImports: true,
			},
		},

		sourcemap: true,
		emptyOutDir: true,
	},
})
