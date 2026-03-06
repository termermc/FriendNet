import { Wunphile } from 'wunphile'

import { NotFoundPage } from './src/component/page/NotFoundPage.ts'
import { HomePage } from './src/component/page/HomePage.ts'
import { ExamplePage } from './src/component/page/ExamplePage.ts'
import { type DocSection, scanDirForDocHierarchy } from './src/util/docs.ts'
import { DocPage } from './src/component/page/DocPage.ts'

const ssg = new Wunphile(import.meta.url)

// Basic pages.
ssg.page('/index.html', HomePage)
ssg.page('/example/index.html', ExamplePage)

// Mount static files.
ssg.staticDir('/', './static')

const rootDocSection = await scanDirForDocHierarchy(
	import.meta.dirname + '/docs',
)
const docsRoot = '/docs'

const mountDocSection = (section: DocSection, pathRelative: string) => {
	if (section.page) {
		let path: string
		if (section.slug === '') {
			path = `${docsRoot}/index.html`
		} else {
			path = `${docsRoot}/${pathRelative}/${section.slug}/index.html`
		}

		ssg.page(path, () =>
			DocPage({
				rootSection: rootDocSection,
				page: section.page,
				docsRoot: docsRoot,
				curRelativePath: pathRelative,
			}),
		)
	}

	for (const child of section.children) {
		mountDocSection(child, `${pathRelative}/${child.slug}`)
	}
}

mountDocSection(rootDocSection, '')

// Set the 404 page.
ssg.notFoundPage(NotFoundPage)

await ssg.cli()
