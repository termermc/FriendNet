import { Wunphile } from 'wunphile'

import { NotFoundPage } from './src/component/page/NotFoundPage.ts'
import { HomePage } from './src/component/page/HomePage.ts'
import { type DocSection, scanDirForDocHierarchy } from './src/util/docs.ts'
import { DocPage } from './src/component/page/DocPage.ts'
import { basename } from 'node:path'

const ssg = new Wunphile(import.meta.url)

// Basic pages.
ssg.page('/index.html', HomePage)

// Mount static files.
ssg.staticDir('/', './static')

const rootDocSection = await scanDirForDocHierarchy(
	import.meta.dirname + '/docs',
)
const docsRoot = '/docs'

const mountDocSection = (section: DocSection, pathRelative: string) => {
	if (section.page) {
		const dir = docsRoot + pathRelative
		const path = dir + '/index.html'

		ssg.page(path, () =>
			DocPage({
				rootSection: rootDocSection,
				section: section,
				docsRoot: docsRoot,
				curRelativePath: pathRelative,
			}),
		)

		for (const filePathFull of section.staticFilePaths) {
			const filename = basename(filePathFull)
			const filePath = filePathFull.substring(import.meta.dirname.length)
			ssg.staticFile(dir + '/' + filename, filePath)
		}
	}

	for (const child of section.children) {
		mountDocSection(child, `${pathRelative}/${child.slug}`)
	}
}

mountDocSection(rootDocSection, '')

// Mount updater dir.
ssg.staticDir('/updater', './updater')

// Set the 404 page.
ssg.notFoundPage(NotFoundPage)

await ssg.cli()
