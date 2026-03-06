import { type Component, html } from 'wunphile'
import { type DocPage as DocPageType } from '../../util/docs.ts'
import { marked } from 'marked'
import { DocsLayout, type DocsLayoutProps } from '../DocsLayout.ts'

export type DocPageProps = {
	/**
	 * The doc page to render.
	 */
	page: DocPageType
} & Omit<DocsLayoutProps, 'title' | 'description'>

/**
 * A documentation page.
 * It is only the page, it does not include the layout.
 */
export const DocPage: Component<DocPageProps, void> = (props) => {
	const { page } = props

	// Render markdown with marked library.
	const contentHtml = marked.parse(page.content, { async: false })

	return DocsLayout(
		{
			...props,
			title: page.title,
			description: page.firstParagraph,
		},
		html`
			<h1 class="doc-page-title">${page.title}</h1>
			<div class="doc-page-content">${html(contentHtml)}</div>
		`,
	)
}
