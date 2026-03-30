import { type Component, html } from 'wunphile'
import { type NewsArticle, newsFileNameNoExtPat } from '../../util/news.ts'
import { NewsLayout, type NewsLayoutProps } from '../NewsLayout.ts'
import { rewriteMarkdownLinks } from '../../util/markdown.ts'
import { marked } from 'marked'
import { formatDate } from '../../util/misc.ts'

export type NewsPageProps = {
	/**
	 * The news article to render.
	 */
	article: NewsArticle
} & Omit<NewsLayoutProps, 'title' | 'description'>

/**
 * A news article page.
 */
export const NewsPage: Component<NewsPageProps, void> = (props) => {
	const { article } = props

	const markdown = rewriteMarkdownLinks(
		article.page.content,
		(label, link) => {
			if (link.startsWith('/')) {
				return null
			}
			if (link.startsWith('http://') || link.startsWith('https://')) {
				return null
			}

			if (link.startsWith('../')) {
				link = link.substring(3)
			}

			let name: string
			let ext: string
			{
				const dotIdx = link.lastIndexOf('.')
				if (dotIdx === -1) {
					name = link
					ext = ''
				} else {
					name = link.substring(0, dotIdx)
					ext = link.substring(dotIdx)
				}
			}

			if (ext === '.md') {
				const match = newsFileNameNoExtPat.exec(name)
				if (match == null) {
					return null
				}

				const [, , slug] = match

				return {
					label: label,
					link: props.newsRoot + '/' + slug + '/',
				}
			}

			return {
				label: label,
				link: props.newsRoot + '/' + props.article.slug + '/' + link,
			}
		},
	)
	const renderContent = html(marked.parse(markdown, { async: false }))

	return NewsLayout(
		{
			...props,
			title: article.page.title,
			description: article.page.firstParagraph,
			newsRoot: '/news',
		},
		html`
			<h1 class="doc-page-title">${article.page.title}</h1>
			<div class="doc-page-date">${formatDate(article.publishDate)}</div>
			<div class="doc-page-content">${renderContent}</div>
		`,
	)
}
