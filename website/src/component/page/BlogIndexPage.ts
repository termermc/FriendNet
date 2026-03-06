import { type Component, html } from 'wunphile'
import type { DocPage } from '../../util/docs.ts'
import { Layout } from '../Layout.ts'

type BlogIndexPageProps = {
	/**
	 * All blog posts.
	 */
	posts: DocPage[]
}

/**
 * The blog index page.
 */
export const BlogIndexPage: Component<BlogIndexPageProps, void> = ({
	posts,
}) => {
	// Sort blog posts by date, newest first.
	const sorted = [...posts].sort(
		(a, b) => b.timestamp.getTime() - a.timestamp.getTime(),
	)

	return Layout(
		{
			title: 'Blog',
			description: `Index of blog posts. There are currently ${posts.length} posts.`,
			stylesheets: ['/css/blog.css'],
		},
		html`
			<h1 class="blog-post-title">Blog Posts</h1>
			<p>There are currently ${posts.length} posts.</p>
			${sorted.map(
				(post) => html`
					<div class="blog-post-listing">
						<a href="/blog/${post.slug}/">${post.title}</a>
						<br />
						<span class="blog-post-meta-date"
							>Posted on
							${post.timestamp.toLocaleDateString()}</span
						>
					</div>
				`,
			)}
		`,
	)
}
