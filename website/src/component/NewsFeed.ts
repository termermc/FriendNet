import config from '../../config.ts'
import { type Component, html } from 'wunphile'
import { type NewsArticle } from '../util/news.ts'

type NewsFeedProps = {
	articles: NewsArticle[]
}

/**
 * The news RSS feed.
 */
export const NewsFeed: Component<NewsFeedProps, void> = ({ articles }) => {
	// The first line must not have any whitespace before it, otherwise it will fail to parse.
	return html`<?xml version="1.0" encoding="UTF-8" ?>
		<rss version="2.0">
			<channel>
				<title>${config.title} News</title>
				<link>${config.prodRootUrl}/news/</link>
				<description>All news posts about ${config.title}.</description>
				${articles.map(
					(article) => html`
					<item>
						<title>${article.page.title}</title>
						<link>${config.prodRootUrl}/news/${article.slug}</link>
						<guid>${article.slug}</guid>
						<pubDate>${article.publishDate.toUTCString()}</pubDate>
						<description>${article.page.firstParagraph}</description>
					</item>
				`,
				)}
			</channel>
		</rss>
	`
}
