import { type Component, html, type RenderFragments } from 'wunphile'
import { BaseLayout, type BaseLayoutProps } from './BaseLayout.ts'
import { type NewsArticle } from '../util/news.ts'
import config from '../../config.ts'

export type NewsLayoutProps = {
	/**
	 * The root URL of the news.
	 * Must not end with a slash.
	 */
	newsRoot: string

	/**
	 * The current path relative to the news root.
	 * Must not start with a slash.
	 * If it is the root of the news, must be an empty string.
	 */
	curRelativePath: string

	/**
	 * All articles.
	 */
	articles: NewsArticle[]
} & Omit<BaseLayoutProps, 'stylesheets'>

/**
 * The layout used for news pages.
 */
export const NewsLayout: Component<NewsLayoutProps, RenderFragments> = (
	props,
	children,
) => {
	return BaseLayout(
		{
			...props,
			stylesheets: ['/css/docs.css?v=' + config.buildTimestamp],
		},
		html`
			<div class="docs">
				<div class="docs-nav">
					<a href="${props.newsRoot}/feed.xml">
						<img
							alt="feed icon"
							src="/feed.svg"
							style="width:1rem"
						/>
						RSS Feed
					</a>
                    <a href="${props.newsRoot}/">All News</a>
					<div>~</div>
					${props.articles
						.slice(0, 10)
						.map(
							(article) => html`
								<a href="${props.newsRoot}/${article.slug}/"
									>${article.page.title}</a
								>
							`,
						)}
				</div>
				<div class="docs-content">${children}</div>
			</div>
		`,
	)
}
