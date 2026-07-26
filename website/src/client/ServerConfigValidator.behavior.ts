import type { BehaviorModule } from 'wunphile'

export default {
	behaviorModuleUrl: import.meta.url,
	behavior: (_) => {
		async function main() {
			const editorCfg = /** @type {typeof import('../../config.ts').default} */ (
				JSON.parse(document.getElementById('editor-config')!.innerText)
			)
			const editorSrcElem = document.getElementById('editor-src')!

			const monaco = (window as unknown as { monaco: typeof import('monaco-editor') }).monaco

			const schemaRes = await fetch(editorCfg.serverConfigSchemaPath)
			if (!schemaRes.ok) {
				console.error('failed to load schema:', schemaRes.status)
				return
			}

			const schema = await schemaRes.json()

			monaco.json.jsonDefaults.setDiagnosticsOptions({
				schemaValidation: 'error',
				schemas: [
					{
						uri: editorCfg.prodRootUrl + editorCfg.serverConfigSchemaPath,
						fileMatch: ['*'],
						schema: schema,
					},
				],
			})
			console.log(schema)

			const container = document.getElementById('editor')!
			container.style.display = 'block'

			monaco.editor.create(container, {
				value: editorSrcElem.innerHTML,
				language: 'json',
				automaticLayout: true,
				theme: 'vs-dark',
			})
			editorSrcElem.style.display = 'none'
		}

		(require as unknown as (deps: string[], func: () => any) => void)(['vs/editor/editor.main'], main)
	},
} satisfies BehaviorModule
