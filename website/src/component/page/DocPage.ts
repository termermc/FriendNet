import { type Component, html, type RenderFragments } from 'wunphile'
import { type DocSection } from '../../util/docs.ts'
import { marked } from 'marked'
import { DocsLayout, type DocsLayoutProps } from '../DocsLayout.ts'
import { basename } from 'node:path'

export type DocPageProps = {
	/**
	 * The section to render.
	 */
	section: DocSection
} & Omit<DocsLayoutProps, 'title' | 'description'>

const mdLinkRegex = /\[([^\]]+)]\(([^)]+)\)/g

/**
 * A documentation page.
 * It is only the page, it does not include the layout.
 */
export const DocPage: Component<DocPageProps, void> = (props) => {
	const { section } = props
	const { page } = section

	let renderContent: RenderFragments

	if (page?.content?.trim()) {
		// Rewrite links.
		const content = page.content.replace(mdLinkRegex, function (substring, label: string, link: string) {
			if (link.startsWith('/')) {
				return substring
			}
			if (link.startsWith('http://') || link.startsWith('https://')) {
				return substring
			}

			let mdDir: string
			if (section.children.length === 0) {
				mdDir = props.curRelativePath.substring(0, props.curRelativePath.lastIndexOf('/'))
			} else {
				mdDir = props.curRelativePath
			}

			let newLink: string
			if (link.endsWith('.md')) {
				const filename = basename(link)
				if (filename === 'index.md' || filename.startsWith('index_')) {
					newLink = mdDir + '/' + link.substring(0, link.lastIndexOf(filename))
				} else {
					newLink = mdDir + '/' + link.substring(0, link.length - '.md'.length) + '/'
				}
			} else {
				newLink = mdDir + '/' + link
			}

			return `[${label}](${props.docsRoot}${newLink})`
		})

		// Render markdown with marked library.
		renderContent = html(marked.parse(content, { async: false }))
	} else if (section.children.length === 0) {
		renderContent = html`
			<i>This section has no content.</i>
		`
	} else {
		renderContent = html`
			Sub-sections:
			<ul>
				${section.children.map((child) => html`
					<li>
						<a href="${props.docsRoot}${props.curRelativePath}/${child.slug}/">${child.slug}</a>
					</li>
				`)}
			</ul>
		`
	}

	return DocsLayout(
		{
			...props,
			title: page.title,
			description: page.firstParagraph,
		},
		html`
			<h1 class="doc-page-title">${page.title}</h1>
			<div class="doc-page-content">${renderContent}</div>
		`,
	)
}
