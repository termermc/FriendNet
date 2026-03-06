import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'

/**
 * The "not found" page.
 */
export const NotFoundPage: Component<void, void> = () => {
	return BaseLayout(
		{
			title: 'Not Found',
			description: 'Page not found',
		},
		html`
			<h1>Not Found</h1>
			<p>There's nothing here for you. <a href="/">Back to home</a></p>
		`,
	)
}
