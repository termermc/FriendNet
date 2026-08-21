import styles from './FileTable.module.css'
import stylesCommon from './common.module.css'

import { Component, createMemo, For, JSX, Show } from 'solid-js'
import { A, useSearchParams } from '@solidjs/router'
import { formatSize, guessFileCategory, trimStrEllipsis } from './util'
import { FileMeta } from '../pb/clientrpc/v1/rpc_pb'
import { FileTableMinPageSize } from './constant'

/**
 * A file to display in {@link FileTable}.
 */
export type FileTableItem<T = void> = {
	/**
	 * The file's metadata.
	 */
	meta: FileMeta

	/**
	 * Extra data included with the file.
	 */
	data: T
}

export type FileTableProps<T> = {
	/**
	 * Whether files are currently loading.
	 */
	isLoading: boolean

	/**
	 * The error message to display, if any.
	 */
	error?: string

	/**
	 * The href of the parent directory, if any.
	 */
	parentHref?: string

	/**
	 * The files to display.
	 * All items will be rendered, even if {@link isLoading} is true.
	 */
	items: FileTableItem<T>[]

	/**
	 * Function that is run for each file.
	 * It returns the options necessary to display the file.
	 */
	forItem: (item: FileTableItem<T>) => {
		/**
		 * Actions markup for the file.
		 */
		actions?: JSX.Element

		/**
		 * Prefix markup for the file.
		 */
		prefix?: JSX.Element
	} & (
		| {
				/**
				 * The file's href.
				 */
				href: string
		  }
		| {
				/**
				 * The function to run when the file is clicked.
				 */
				onClick: () => void
		  }
	)
}

/**
 * FileTable displays a list of files in a table.
 */
export const FileTable = (<T,>(props: FileTableProps<T>) => {
	const [search, setSearch] = useSearchParams<{
		page: string | undefined
		pageSize: string | undefined
	}>()

	const pageSize = createMemo(() =>
		Math.max(parseInt(search.pageSize ?? '0', 10), FileTableMinPageSize),
	)
	const totalPages = createMemo(() =>
		Math.ceil(props.items.length / pageSize()),
	)
	const page = createMemo(() =>
		Math.min(Math.max(parseInt(search.page ?? '0', 10), 0), totalPages()),
	)
	const visibleItems = createMemo(() =>
		props.items.slice(page() * pageSize(), (page() + 1) * pageSize()),
	)

	const pageSizes = [250, 500, 750, 1000]

	return (
		<div class={styles.files}>
			<Show when={props.items.length > FileTableMinPageSize}>
				<div class={styles.pagination}>
					<button
						onClick={() =>
							setSearch({ page: (page() - 1).toString() })
						}
						disabled={page() === 0}
					>
						⤶
					</button>
					<span class={styles.paginationPage}>
						Page{' '}
						<input
							type="number"
							value={page() + 1}
							onInput={(e) => {
								const value = parseInt(
									e.currentTarget.value,
									10,
								)
								if (
									!isNaN(value) &&
									value >= 1 &&
									value <= totalPages()
								) {
									setSearch({ page: (value - 1).toString() })
								}
							}}
						/>{' '}
						of {totalPages()}
					</span>
					<button
						onClick={() =>
							setSearch({ page: (page() + 1).toString() })
						}
						disabled={page() === totalPages() - 1}
					>
						⤷
					</button>{' '}
					<select
						onChange={(e) =>
							setSearch({
								page: 0,
								pageSize: e.currentTarget.value,
							})
						}
						value={pageSize()}
					>
						{pageSizes.map((size) => (
							<option value={size}>Show {size} files</option>
						))}
					</select>
				</div>

				<div class={styles.paginationSpacer} />
			</Show>
			<table>
				<thead>
					<tr>
						<th>File</th>
						<th>Size</th>
						<th>Actions</th>
					</tr>
				</thead>
				<tbody>
					<Show when={props.isLoading}>
						<tr>
							<td colSpan="2">Loading...</td>
						</tr>
					</Show>
					<Show when={props.error}>
						<tr>
							<td colSpan="2" class={stylesCommon.errorMessage}>
								{props.error}
							</td>
						</tr>
					</Show>

					<Show when={props.parentHref}>
						<tr>
							<td>
								<A
									href={props.parentHref!}
									title="Up a directory"
									classList={{
										[stylesCommon.w100]: true,
										[stylesCommon.displayInlineBlock]: true,
									}}
								>
									▲ ..
								</A>
							</td>
						</tr>
					</Show>
					<For each={visibleItems()}>
						{(item) => {
							const meta = item.meta

							let emoji: string
							if (meta.isDir) {
								emoji = '📁'
							} else {
								const [cat] = guessFileCategory(meta.name)
								switch (cat) {
									case 'text':
										emoji = '📜'
										break
									case 'image':
										emoji = '🖼️'
										break
									case 'video':
										emoji = '🎞️'
										break
									case 'audio':
										emoji = '🎵'
										break
									case 'rich':
										emoji = '🖨️'
										break
									case 'other':
										emoji = '📄'
										break
								}
							}

							const label = trimStrEllipsis(
								emoji + ' ' + meta.name,
								100,
							)

							const options = props.forItem(item)

							return (
								<tr>
									{'href' in options ? (
										<td
											title={meta.name}
											class={styles.label}
										>
											<A href={options.href}>
												{options.prefix}
												{label}
											</A>
										</td>
									) : (
										<td
											title={meta.name}
											onClick={options.onClick}
											class={styles.label}
										>
											<span>
												{options.prefix}
												{label}
											</span>
										</td>
									)}
									<td class={styles.sizeTd}>
										{item.meta.isDir
											? ''
											: formatSize(
													Number(item.meta.size),
													2,
												)}
									</td>
									<td class={styles.actionsTd}>
										<div class={styles.actions}>
											{options.actions}
										</div>
									</td>
								</tr>
							)
						}}
					</For>
				</tbody>
			</table>
		</div>
	)
}) satisfies Component<FileTableProps<any>>
