import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { Layout } from '../Layout.ts'

/**
 * An example page without much content.
 */
export const ExamplePage: Component<void, void> = () => {
	return Layout(
		{
			title: 'Example',
			description: 'Example page. Not much to see here.',
		},
		html`
			<h1>Example Page</h1>
			<p>This is my example page. WOW!</p>
			<p>
				This is boring. Go read the <a href="/docs/">docs</a> instead.
			</p>
		`,
	)
}
