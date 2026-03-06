import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'

/**
 * The homepage.
 */
export const HomePage: Component<void, void> = () => {
	return BaseLayout(
		{
			description:
				'Self-hostable file sharing for friends, like a mini-Soulseek. No port forwarding needed!',
			stylesheets: ['/css/home.css'],
		},
		html`
			<h1>Welcome</h1>
			<p>
				Why not take a look at the <a href="/example/">example page</a>?
			</p>
		`,
	)
}
