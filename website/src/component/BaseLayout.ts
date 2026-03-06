import config from '../../config.ts'

import { type Component, type RenderFragments, html } from 'wunphile'

export type BaseLayoutProps = {
	/**
	 * The page title.
	 * Optional.
	 * If omitted, uses the site title only.
	 */
	title?: string

	/**
	 * The page description.
	 * Optional.
	 */
	description?: string

	/**
	 * URIs to any additional CSS files to include in the page.
	 * Optional.
	 */
	stylesheets?: string[]

	/**
	 * URIs to any additional JS files to include in the page.
	 * Optional.
	 */
	scripts?: string[]
}

/**
 * The main page layout.
 */
export const BaseLayout: Component<BaseLayoutProps, RenderFragments> = (
	{ title, description, stylesheets, scripts },
	children,
) => {
	let titleRes: string
	if (title) {
		titleRes = `${title} - ${config.title}`
	} else {
		titleRes = config.title
	}

	return html`
		<!DOCTYPE html>
		<html lang="en">
			<head>
				<meta charset="UTF-8" />
				<meta
					name="viewport"
					content="width=device-width, initial-scale=1.0"
				/>

				<title>${titleRes}</title>
				<meta property="og:title" content="${titleRes}" />

				${description
					? html`
							<meta
								property="og:description"
								content="${description}"
							/>
						`
					: ''}

				<link rel="stylesheet" href="/css/main.css" />
				${(stylesheets ?? []).map(
					(uri) => html` <link rel="stylesheet" href="${uri}" /> `,
				)}
			</head>
			<body>
				<header>
					<a href="/" class="header-title">${config.title}</a>
					<div class="header-nav">
						<a href="/download/" class="header-nav-item"
							>Download</a
						>
						<a href="/docs/" class="header-nav-item"
							>Documentation</a
						>
						<a href="${config.githubUrl}" class="header-nav-item"
							>GitHub</a
						>
					</div>
				</header>
				<main>${children}</main>
				${(scripts ?? []).map(
					(uri) => html`<script src="${uri}"></script>`,
				)}
			</body>
		</html>
	`
}
