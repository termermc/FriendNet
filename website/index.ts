import { Wunphile } from 'wunphile'

import { type DocSection, scanDirForDocHierarchy } from './src/util/docs.ts'
import type { UpdateInfo } from './update.ts'

import { NotFoundPage } from './src/component/page/NotFoundPage.ts'
import { HomePage } from './src/component/page/HomePage.ts'
import { DocPage } from './src/component/page/DocPage.ts'
import { basename, join } from 'node:path'
import { ScreenshotsPage } from './src/component/page/ScreenshotsPage.ts'
import { readFile } from "node:fs/promises";
import {DownloadPage} from "./src/component/page/DownloadPage.ts";

const ssg = new Wunphile(import.meta.url)

// Basic pages.
ssg.page('/index.html', HomePage)
ssg.page('/screenshots/index.html', ScreenshotsPage)

// Read current update info and mount download page.
let curUpdate: UpdateInfo
{
    const json = await readFile(join(import.meta.dirname, 'updater', 'client', 'latest.json'), 'utf-8')
    curUpdate = JSON.parse(json)
}

ssg.page('/download/index.html', () => DownloadPage({ curUpdate }))

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
