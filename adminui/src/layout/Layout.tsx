import { Component, ErrorBoundary, JSX } from 'solid-js'

import styles from './Layout.module.css'

import { AppName } from '../constant'
import { A } from '@solidjs/router'

type LayoutProps = {
	children: JSX.Element
}

export const Layout: Component<LayoutProps> = (props) => {
	return (
		<div class={styles.container}>
			<header>
				<span class={styles.headerTitle}>{AppName}</span>

				<div class={styles.options}>
					<A
						href="/todo"
						class={styles.option}
					>
						⏳ TODO
					</A>{' '}
					<A href="/todo" class={styles.option}>
						🔧 TODO
					</A>
				</div>
			</header>

			<main>
				<div class={styles.sidebar}>
					TODO Rooms browser
				</div>

				<div class={styles.content}>
					<ErrorBoundary
						fallback={(err) => {
							console.log('failed to render page content:', err)

							return (
								<div class={styles.errorContainer}>
									<h1>Failed to render page content</h1>
									<div class={styles.error}>
										{err?.message ?? err}
									</div>
									<p>See console for details.</p>
								</div>
							)
						}}
					>
						{props.children}
					</ErrorBoundary>
				</div>
			</main>
		</div>
	)
}
