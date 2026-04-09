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

	let descRes: string
	if (description) {
		descRes = description
	} else {
		descRes =
			'Self-hostable file sharing for friends, like a mini-Soulseek. No port forwarding needed!'
	}

	// noinspection JSUnresolvedLibraryURL
	return html`
		<!DOCTYPE html>
		<html lang="en">
			<head>
				<meta charset="UTF-8" />
				<meta
					name="viewport"
					content="width=device-width, initial-scale=1.0"
				/>
				<link rel="icon" href="/favicon.png" />

				<title>${titleRes}</title>
				<meta property="og:title" content="${titleRes}" />

				<meta property="og:description" content="${descRes}" />

				<meta property="og:image" content="/logo-full.png" />

				<link
					rel="stylesheet"
					href="/css/main.css?v=${config.buildTimestamp}"
				/>
				${(stylesheets ?? []).map(
					(uri) => html` <link rel="stylesheet" href="${uri}" /> `,
				)}
			</head>
			<body>
				<header>
					<a href="/" class="header-title">
						<img src="/logo-full.png" alt="logo" />
						<span>${config.title}</span>
					</a>
					<div class="header-nav">
						<a href="/" class="header-nav-item"> About </a>
						<a href="/download/" class="header-nav-item">
							Download
						</a>
						<a href="/screenshots/" class="header-nav-item">
							Screenshots
						</a>
						<a href="/news/" class="header-nav-item"> News </a>
						<a href="/docs/" class="header-nav-item">
							Documentation
						</a>
						<a href="${config.githubUrl}" class="header-nav-item">
							GitHub
						</a>
						<a href="/donate/" class="header-nav-item">
                            ❤️ Donate
						</a>
					</div>
				</header>
				<main>${children}</main>
				${(scripts ?? []).map(
					(uri) => html`<script src="${uri}"></script>`,
				)}
				<script
					defer
					data-domain="friendnet.org"
					src="https://curiosity.termer.net/js/script.file-downloads.outbound-links.js"
				></script>
				<script>
					window.plausible =
						window.plausible ||
						function () {
							;(window.plausible.q =
								window.plausible.q || []).push(arguments)
						}
				</script>
			</body>
		</html>
	`
}
