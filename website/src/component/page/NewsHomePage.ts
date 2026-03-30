import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'
import config from '../../../config.ts'
import { type NewsArticle } from '../../util/news.ts'
import { marked } from 'marked'
import { formatDate } from '../../util/misc.ts'

type NewsHomePageProps = {
	newsRoot: string
	articles: NewsArticle[]
}

/**
 * The news homepage.
 */
export const NewsHomePage: Component<NewsHomePageProps, void> = ({
	newsRoot,
	articles,
}) => {
	return BaseLayout(
		{
			title: 'News',
			stylesheets: ['/css/home.css?v=' + config.buildTimestamp],
		},
		html`
			<div class="home">
				<h1>News</h1>
				<div class="home-content news">
					<p>
						All news posts about ${config.title}.

						<a href="${newsRoot}/feed.xml">
							<img
								src="/feed.svg"
								style="width:1rem"
								alt="feed icon"
							/>
							RSS feed
						</a>
					</p>

					<hr />

					${articles.map((article) => {
						const url = `${newsRoot}/${article.slug}/`

						return html`
							<div class="article">
								<a href="${url}" class="article-title"
									>${article.page.title}</a
								>
								<div class="article-date">
									${formatDate(article.publishDate)}
								</div>
								<div class="article-content">
									${html(
										marked.parse(
											article.page.firstParagraph,
											{ async: false },
										),
									)}
								</div>
								<a href="${url}">(Read more)</a>
							</div>
						`
					})}
				</div>
			</div>
		`,
	)
}
