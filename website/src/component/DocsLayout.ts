import { type Component, html, type RenderFragments } from 'wunphile'
import { type DocSection } from '../util/docs.ts'
import { Layout, type LayoutProps } from './Layout.ts'

export type DocsLayoutProps = {
	/**
	 * The root URL of the docs.
	 * Must not end with a slash.
	 */
	docsRoot: string

	/**
	 * The root section of the docs.
	 */
	rootSection: DocSection

	/**
	 * The current path relative to the docs root.
	 * Must not start with a slash.
	 * If it is the root of the docs, must be an empty string.
	 */
	curRelativePath: string
} & LayoutProps

/**
 * The layout used for documentation pages.
 */
export const DocsLayout: Component<DocsLayoutProps, RenderFragments> = (
	props,
	children,
) => {
	const mkSection = (
		section: DocSection,
		relativePath: string,
	): RenderFragments => {
		const isOpen = props.curRelativePath.startsWith(relativePath)

		return html`
			<details class="docs-nav-section" ${isOpen ? 'open' : ''}>
				<summary>
					${section.page
						? html`
								<a href="${props.docsRoot}/${relativePath}/">
									${section.page.title}
								</a>
							`
						: html` <span> ${section.slug} </span> `}
				</summary>
				<div class="docs-nav-section-children">
					${section.children.map((child) =>
						mkSection(child, `${relativePath}/${child.slug}`),
					)}
				</div>
			</details>
		`
	}

	return Layout(
		props,
		html`
			<div class="docs-nav">${mkSection(props.rootSection, '')}</div>
			<div class="docs-content">${children}</div>
		`,
	)
}
