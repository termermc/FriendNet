import { type MarkdownPage, readMarkdownPage } from './markdown.ts'
import { readdir } from 'node:fs/promises'

/**
 * A news article.
 */
export type NewsArticle = {
	/**
	 * The article's slug.
	 */
	slug: string

	/**
	 * The article's publish date.
	 */
	publishDate: Date

	/**
	 * The article's page.
	 */
	page: MarkdownPage

	/**
	 * The paths to static files in the section.
	 * These must be absolute paths.
	 */
	staticFilePaths: string[]

	/**
	 * The article's containing directory, if any.
	 * This will only be set if the article is in its own directory, as opposed to a freestanding markdown file.
	 */
	containingDir?: string
}

/**
 * The pattern for news article filenames, without the extension.
 */
export const newsFileNameNoExtPat = /^(\d{12})_(.+)$/

/**
 * Scans a directory for news articles.
 * The returned articles array will be sorted by publish date, descending.
 * @param dirPath The directory to scan.
 * @returns The articles in the directory.
 */
export async function scanDirForNews(dirPath: string): Promise<NewsArticle[]> {
	const articles: NewsArticle[] = []

	const rootFiles = await readdir(dirPath)
	for (const name of rootFiles) {
		if (name.startsWith('.')) {
			continue
		}

		let ext: string
		let noExt: string
		{
			const dotIdx = name.lastIndexOf('.')
			if (dotIdx === -1) {
				ext = ''
				noExt = name
			} else {
				ext = name.substring(dotIdx)
				noExt = name.substring(0, dotIdx)
			}
		}

		if (ext !== '.md' && ext !== '') {
			throw new Error(
				`found filename in news dir that was not a directory or markdown: ${name}`,
			)
		}

		// Parse name.
		const match = newsFileNameNoExtPat.exec(noExt)
		if (!match) {
			throw new Error(
				`found filename in news dir that did not match expected format: ${name}`,
			)
		}
		const [_, dateStr, slug] = match

		// Parse date.
		const date = new Date(
			`${dateStr.substring(0, 4)}-${dateStr.substring(4, 6)}-${dateStr.substring(6, 8)} ${dateStr.substring(8, 10)}:${dateStr.substring(10, 12)}Z`,
		)
		if (isNaN(date.getTime())) {
			throw new Error(
				`found filename in news dir that did not match expected format: ${name}`,
			)
		}

		const staticFilePaths: string[] = []
		let page: MarkdownPage
		let containingDir: string | undefined

		if (ext === '') {
			// File is a directory; search for index and files.

			containingDir = name

			const subFiles = await readdir(`${dirPath}/${name}`)

			for (const subName of subFiles) {
				const subPath = `${dirPath}/${name}/${subName}`

				let subExt: string
				{
					const dotIdx = subName.lastIndexOf('.')
					if (dotIdx === -1) {
						subExt = ''
					} else {
						subExt = subName.substring(dotIdx)
					}
				}

				const isIndex =
					subName === 'index.md' ||
					(subName.startsWith('index_') && subExt === '.md')
				if (isIndex) {
					page = await readMarkdownPage(subPath)
					continue
				}

				if (subExt === '.md') {
					throw new Error(
						`found markdown file in news dir that was not an index: ${subName}`,
					)
				}

				staticFilePaths.push(subPath)
			}

			if (!page) {
				throw new Error(
					`found directory in news dir that did not contain an index: ${name}`,
				)
			}
		} else {
			// File is an article itself.
			page = await readMarkdownPage(`${dirPath}/${name}`)
		}

		articles.push({
			slug: slug,
			publishDate: date,
			page: page,
			staticFilePaths: staticFilePaths,
			containingDir: containingDir,
		})
	}

	articles.sort((a, b) => b.publishDate.getTime() - a.publishDate.getTime())
	return articles
}
