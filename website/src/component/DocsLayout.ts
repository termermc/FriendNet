import { type Component, html, type RenderFragments } from 'wunphile'
import { type DocSection } from '../util/docs.ts'
import { BaseLayout, type BaseLayoutProps } from './BaseLayout.ts'

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
} & Omit<BaseLayoutProps, 'stylesheets'>

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
		const isCur = props.curRelativePath === relativePath

		let label: RenderFragments
		if (section.page) {
			label = html`
				<a href="${props.docsRoot}${relativePath}/">
					${section.page.title}
				</a>
			`
		} else {
			label = html`<span>${section.slug}</span>`
		}

		if (section.children.length > 0) {
			return html`
				<details
					class="docs-nav-section ${isCur
						? 'docs-nav-section-current'
						: ''}"
					${isOpen ? 'open' : ''}
				>
					<summary class="docs-nav-section-label">${label}</summary>
					<div class="docs-nav-section-children">
						${section.children.map((child) =>
							mkSection(child, `${relativePath}/${child.slug}`),
						)}
					</div>
				</details>
			`
		} else {
			return html`
				<div
					class="docs-nav-section ${isCur
						? 'docs-nav-section-current'
						: ''}"
					${isOpen ? 'open' : ''}
				>
					<span class="docs-nav-section-label">${label}</span>
				</div>
			`
		}
	}

	return BaseLayout(
		{
			...props,
			stylesheets: ['/css/docs.css'],
		},
		html`
			<div class="docs">
				<div class="docs-nav">${mkSection(props.rootSection, '')}</div>
				<div class="docs-content">${children}</div>
			</div>
		`,
	)
}
