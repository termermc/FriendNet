import {
	Component,
	createMemo,
	createSignal,
	For,
	onMount,
	Show,
} from 'solid-js'
import styles from './SearchFiltersPage.module.css'
import stylesCommon from '../common.module.css'
import {
	BlacklistMatchMode,
	BlacklistPolicy,
} from '../../pb/serverrpc/v1/rpc_pb'
import { useLocation, useParams } from '@solidjs/router'
import { usePageRoom, useRpcClient } from '../ctx'
import { RoomLoader } from '../RoomLoader'
import { createStore } from 'solid-js/store'

const matchModeToName = (() => {
	const res = new Array<string>(4).fill('')
	res[BlacklistMatchMode.UNSPECIFIED] = '(Unknown)'
	res[BlacklistMatchMode.SUBSTRING] = '🔠 Substring'
	res[BlacklistMatchMode.WHOLE] = '💬 Whole Word'
	res[BlacklistMatchMode.REGEX] = '⚙️ Regex'
	return res
})()

type Delta = {
	isAdd: boolean
	mode: BlacklistMatchMode
	keyword: string
}

type Filter = {
	keyword: string
	mode: BlacklistMatchMode
	isDeleted: boolean
	isNew: boolean
	isTombstone: boolean
}

const Page: Component = () => {
	const room = usePageRoom()
	const client = useRpcClient()

	const [deltas, setDeltas] = createStore<Delta[]>([])
	const [filters, setFilters] = createStore<Filter[]>([])
	const [isLoading, setLoading] = createSignal(false)
	const [error, setError] = createSignal('')
	const [isApplying, setApplying] = createSignal(false)

	const deltaStats = createMemo(() => {
		let add = 0
		let del = 0
		for (const delta of deltas) {
			if (delta.isAdd) {
				add++
			} else {
				del++
			}
		}

		return { add, del }
	})

	const refreshFilters = async () => {
		try {
			setLoading(true)

			const { policies } = await client.getBlacklistPolicies({
				room: room?.name,
			})

			const f = new Array<Filter>(policies.length)
			for (let i = 0; i < f.length; i++) {
				const { keyword, mode } = policies[i]
				f[i] = {
					keyword,
					mode,
					isDeleted: false,
					isNew: false,
					isTombstone: false,
				}
			}

			setFilters(f)
		} catch (err) {
			console.error('failed to load blacklist:', err)
		} finally {
			setLoading(false)
		}
	}
	onMount(refreshFilters)

	const formMode = 'filter-mode'
	const formKeyword = 'filter-keyword'
	const onAdd = (e: SubmitEvent) => {
		e.preventDefault()
		const mode = parseInt(
			(document.getElementsByName(formMode)[0] as HTMLSelectElement)
				.value,
		) as BlacklistMatchMode
		const keywordElem = document.getElementsByName(
			formKeyword,
		)[0] as HTMLInputElement
		const keyword = keywordElem.value.toLowerCase()

		setError('')

		if (keyword === '') {
			return
		}

		// Check if word already exists.
		for (const filter of filters) {
			if (filter.isTombstone || filter.isDeleted) {
				continue
			}
			if (filter.keyword === keyword) {
				setError(
					`There is already a filter for this keyword: ${keyword}`,
				)
				return
			}
		}

		setDeltas(deltas.length, {
			isAdd: true,
			mode,
			keyword,
		})
		setFilters(filters.length, {
			keyword,
			mode,
			isDeleted: false,
			isNew: true,
			isTombstone: false,
		} satisfies Filter)

		keywordElem.value = ''
	}

	const revertDeltas = () => {
		for (let i = 0; i < filters.length; i++) {
			const filter = filters[i]
			if (filter.isTombstone) {
				return
			}

			for (let j = 0; j < deltas.length; j++) {
				const delta = deltas[j]

				if (delta.keyword !== filter.keyword) {
					continue
				}

				if (delta.isAdd) {
					setFilters(i, {
						isTombstone: true,
					})
				} else if (!delta.isAdd) {
					setFilters(i, {
						isDeleted: false,
					})
				}
			}
		}

		setDeltas([])
	}
	const applyDeltas = async () => {
		let addOk = false
		let delOk = false

		try {
			setApplying(true)
			setError('')

			const adds: Omit<BlacklistPolicy, '$typeName'>[] = []
			const dels: string[] = []
			deltaLoop: for (const delta of deltas) {
				if (delta.isAdd) {
					// Don't add if there's also a del for this keyword.
					for (const sub of deltas) {
						if (!sub.isAdd && sub.keyword === delta.keyword) {
							continue deltaLoop
						}
					}

					adds.push({
						keyword: delta.keyword,
						mode: delta.mode,
					})
				} else {
					// Don't del if there's also an add for this keyword.
					for (const sub of deltas) {
						if (sub.isAdd && sub.keyword === delta.keyword) {
							continue deltaLoop
						}
					}

					dels.push(delta.keyword)
				}
			}

			// Deletes must be committed first to avoid conflicts.
			if (dels.length > 0) {
				await client.removeBlacklistPolicies({
					room: room?.name,
					policies: dels,
				})
				delOk = true
			} else {
				delOk = true
			}
			if (adds.length > 0) {
				await client.addBlacklistPolicies({
					room: room?.name,
					policies: adds,
				})
			} else {
				addOk = true
			}

			// All is well; reset deltas and change filter states accordingly.
			setDeltas([])
			for (let i = 0; i < filters.length; i++) {
				const filter = filters[i]
				if (filter.isTombstone) {
					continue
				}

				let shouldChange = false
				const diff: Partial<Filter> = {}
				if (filter.isDeleted) {
					diff.isTombstone = true
					shouldChange = true
				} else if (filter.isNew) {
					diff.isNew = false
					shouldChange = true
				}

				if (shouldChange) {
					setFilters(i, diff)
				}
			}
		} catch (err) {
			if ((addOk && !delOk) || (!addOk && delOk)) {
				// Reload filters because it was only a partial success.
				revertDeltas()
				void refreshFilters()
			}

			console.error('failed to apply deltas:', err)
			setError(
				'Failed to apply changes: ' +
					((err as any).message || String(err)),
			)
		} finally {
			setApplying(false)
		}
	}

	return (
		<div class={styles.container}>
			<Show
				when={room}
				fallback={
					<>
						<h1>Global Search Filters</h1>
						<p>
							These filters apply to searches in all rooms on the
							server.
						</p>
					</>
				}
			>
				<h1>Search Filters For {room!.name}</h1>
				<p>These filters only apply to searches in {room!.name}.</p>
			</Show>

			<form class={stylesCommon.form} onSubmit={onAdd}>
				<table>
					<thead>
						<tr>
							<th>Match Mode</th>
							<th>Keyword</th>
							<th>Action</th>
						</tr>
					</thead>
					<tbody>
						<For each={filters}>
							{(filter, i) => (
								<Show when={!filter.isTombstone}>
									<tr
										classList={{
											[styles.filter]: true,
											[styles.deleted]: filter.isDeleted,
											[styles.new]: filter.isNew,
										}}
									>
										<td>{matchModeToName[filter.mode]}</td>
										<td>
											<code>
												{filter.keyword}
											</code>
										</td>
										<td>
											<button
												type="button"
												onClick={() => {
													if (filter.isDeleted) {
														return
													}

													setFilters(i(), {
														isDeleted: true,
													})
													setDeltas(deltas.length, {
														isAdd: false,
														keyword: filter.keyword,
														mode: filter.mode,
													} satisfies Delta)
												}}
											>
												🗑️
											</button>
										</td>
									</tr>
								</Show>
							)}
						</For>

						<Show
							when={isLoading() && !error()}
							fallback={
								<>
									<tr>
										<td>
											<select
												name={formMode}
												value={
													BlacklistMatchMode.SUBSTRING
												}
												disabled={isApplying()}
											>
												<For
													each={matchModeToName.slice(
														1,
													)}
												>
													{(name, mode) => (
														<option
															value={mode() + 1}
														>
															{name}
														</option>
													)}
												</For>
											</select>
										</td>
										<td>
											<input
												type="text"
												name={formKeyword}
												disabled={isApplying()}
											/>
										</td>
										<td>
											<input
												type="submit"
												value="Add"
												disabled={isApplying()}
											/>
										</td>
									</tr>

									<Show when={deltas.length > 0}>
										<tr>
											<td>Unsaved Changes</td>
											<td>
												<span style="color:green">
													+{deltaStats().add}
												</span>{' '}
												<span style="color:red">
													-{deltaStats().del}
												</span>
											</td>
											<td>
												<button
													type="button"
													onClick={revertDeltas}
													disabled={isApplying()}
												>
													❌ Revert
												</button>
												<button
													type="button"
													onClick={applyDeltas}
													disabled={isApplying()}
												>
													✅ Apply
												</button>
											</td>
										</tr>
									</Show>
								</>
							}
						>
							<tr>
								<td>Loading...</td>
							</tr>
						</Show>
					</tbody>
				</table>
			</form>

			<Show when={error()}>
				<div class={stylesCommon.errorMessage}>{error()}</div>
			</Show>
		</div>
	)
}

export const RoomSearchFiltersPage: Component = () => {
	const loc = useLocation()
	const params = useParams<{ name: string }>()

	return (
		<Show when={loc.pathname} keyed>
			<RoomLoader room={params.name}>
				<Page />
			</RoomLoader>
		</Show>
	)
}

export const GlobalSearchFiltersPage = Page
