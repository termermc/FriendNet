import styles from './LogsPage.module.css'

import {
	Component,
	For,
	Show,
	createEffect,
	createSignal,
	on,
	onMount,
} from 'solid-js'
import { useGlobalState } from '../ctx'
import { LogMessage } from '../../pb/clientrpc/v1/rpc_pb'
import { collect } from '../util'

const INITIAL_BATCH = 200
const LOAD_BATCH = 200
const MAX_DOM_ITEMS = 400
const SCROLL_THRESHOLD_PX = 200
const AUTO_SCROLL_THRESHOLD_PX = 120

type LogMsgProps = {
	msg: LogMessage
	formatAttrs: (msg: LogMessage) => string
}

const LogMsg: Component<LogMsgProps> = (props) => {
	const timestamp = () => new Date(Number(props.msg.createdTs)).toLocaleString()
	const attrsText = () => props.formatAttrs(props.msg)

	return (
		<div class={styles.item}>
			<div class={styles.itemHeader}>
				<span class={styles.timestamp}>{timestamp()}</span>
				<span class={styles.message}>{props.msg.message}</span>
			</div>
			<Show when={props.msg.attrs.length > 0}>
				<div class={styles.attrs}>{attrsText()}</div>
			</Show>
		</div>
	)
}

type UpdateMode = 'preserve' | 'stickBottom' | 'none'

type TrimResult = {
	logs: LogMessage[]
	trimmedTop: boolean
}

export const LogsPage: Component = () => {
	const state = useGlobalState()

	const [logs, setLogs] = createSignal<LogMessage[]>([])
	const [hasOlder, setHasOlder] = createSignal(true)
	const [isLoadingTop, setIsLoadingTop] = createSignal(false)
	const [isLoadingBottom, setIsLoadingBottom] = createSignal(false)

	let containerElem: HTMLDivElement | undefined

	const formatAttrs = (msg: LogMessage): string => {
		if (msg.attrs.length === 0) {
			return ''
		}

		const attrsObj = state.log.attrsToObject(msg.attrs)
		const parts: string[] = []
		for (const [key, val] of Object.entries(attrsObj)) {
			parts.push(`${key}=${JSON.stringify(val)}`)
		}
		return parts.join(' ')
	}

	const isNearTop = () => {
		if (!containerElem) {
			return false
		}
		return containerElem.scrollTop <= SCROLL_THRESHOLD_PX
	}

	const isNearBottom = (threshold = SCROLL_THRESHOLD_PX) => {
		if (!containerElem) {
			return false
		}
		return (
			containerElem.scrollHeight -
				(containerElem.scrollTop + containerElem.clientHeight) <=
			threshold
		)
	}

	const scrollToBottom = () => {
		if (!containerElem) {
			return
		}

		containerElem.scrollTop =
			containerElem.scrollHeight - containerElem.clientHeight
	}

	const updateLogs = (nextLogs: LogMessage[], mode: UpdateMode) => {
		const container = containerElem
		let prevScrollTop = 0
		let prevScrollHeight = 0

		if (container && mode === 'preserve') {
			prevScrollTop = container.scrollTop
			prevScrollHeight = container.scrollHeight
		}

		setLogs(nextLogs)

		if (!container) {
			return
		}

		requestAnimationFrame(() => {
			if (!containerElem) {
				return
			}

			if (mode === 'preserve') {
				const newScrollHeight = containerElem.scrollHeight
				const delta = newScrollHeight - prevScrollHeight
				if (delta !== 0) {
					containerElem.scrollTop = prevScrollTop + delta
				}
			} else if (mode === 'stickBottom') {
				scrollToBottom()
			}
		})
	}

	const trimToMax = (
		list: LogMessage[],
		trimFrom: 'top' | 'bottom',
	): TrimResult => {
		if (list.length <= MAX_DOM_ITEMS) {
			return { logs: list, trimmedTop: false }
		}

		const excess = list.length - MAX_DOM_ITEMS
		if (trimFrom === 'top') {
			return {
				logs: list.slice(excess),
				trimmedTop: true,
			}
		}

		return {
			logs: list.slice(0, list.length - excess),
			trimmedTop: false,
		}
	}

	const makeUidSet = (list: LogMessage[]): Set<string> => {
		const res = new Set<string>()
		for (const msg of list) {
			res.add(msg.uid)
		}
		return res
	}

	const iterateOlder = function* (
		beforeTs: bigint,
		existingUids: Set<string>,
	): Generator<LogMessage, void, void> {
		const gen = state.log.iterateBefore(new Date(Number(beforeTs)))
		for (const msg of gen) {
			if (msg.createdTs > beforeTs) {
				continue
			}
			if (existingUids.has(msg.uid)) {
				continue
			}
			yield msg
		}
	}

	const iterateNewer = function* (
		afterTs: bigint,
		existingUids: Set<string>,
	): Generator<LogMessage, void, void> {
		const gen = state.log.iterateAfter(new Date(Number(afterTs)))
		for (const msg of gen) {
			if (msg.createdTs < afterTs) {
				continue
			}
			if (existingUids.has(msg.uid)) {
				continue
			}
			yield msg
		}
	}

	const loadInitial = () => {
		const beforeTs = BigInt(Date.now()) + 1n
		const batch = collect(iterateOlder(beforeTs, new Set()), INITIAL_BATCH)
		batch.reverse()
		setLogs(batch)
		setHasOlder(batch.length === INITIAL_BATCH)
		requestAnimationFrame(() => scrollToBottom())
	}

	const loadOlder = () => {
		if (isLoadingTop() || !hasOlder()) {
			return
		}

		setIsLoadingTop(true)

		const current = logs()
		const beforeTs =
			current.length > 0 ? current[0].createdTs : BigInt(Date.now())
		const existingUids = makeUidSet(current)
		const batch = collect(
			iterateOlder(beforeTs, existingUids),
			LOAD_BATCH,
		)

		if (batch.length === 0) {
			setHasOlder(false)
			setIsLoadingTop(false)
			return
		}

		batch.reverse()
		let next = [...batch, ...current]
		const trimmed = trimToMax(next, 'bottom')
		updateLogs(trimmed.logs, 'preserve')

		setIsLoadingTop(false)
	}

	const loadNewer = () => {
		if (isLoadingBottom()) {
			return
		}

		setIsLoadingBottom(true)

		const current = logs()
		const afterTs =
			current.length > 0
				? current[current.length - 1].createdTs
				: 0n
		const existingUids = makeUidSet(current)
		const batch = collect(
			iterateNewer(afterTs, existingUids),
			LOAD_BATCH,
		)

		if (batch.length === 0) {
			setIsLoadingBottom(false)
			return
		}

		const shouldStickBottom = isNearBottom(AUTO_SCROLL_THRESHOLD_PX)
		const trimFrom = shouldStickBottom ? 'top' : 'bottom'
		let next = [...current, ...batch]
		const trimmed = trimToMax(next, trimFrom)
		const mode: UpdateMode = shouldStickBottom
			? 'stickBottom'
			: trimmed.trimmedTop
				? 'preserve'
				: 'none'
		updateLogs(trimmed.logs, mode)
		if (trimmed.trimmedTop) {
			setHasOlder(true)
		}

		setIsLoadingBottom(false)
	}

	const onScroll = () => {
		if (isNearTop()) {
			loadOlder()
		}

		if (isNearBottom()) {
			loadNewer()
		}
	}

	onMount(() => {
		loadInitial()
	})

	createEffect(
		on(
			() => state.log.latestLog(),
			(latest) => {
				if (!latest) {
					return
				}

				const current = logs()
				if (current.length > 0) {
					const last = current[current.length - 1]
					if (last.uid === latest.uid) {
						return
					}
				}

				if (current.some((msg) => msg.uid === latest.uid)) {
					return
				}

				const shouldStickBottom = isNearBottom(AUTO_SCROLL_THRESHOLD_PX)
				const trimFrom = shouldStickBottom ? 'top' : 'bottom'
				let next = [...current, latest]
				const trimmed = trimToMax(next, trimFrom)
				const mode: UpdateMode = shouldStickBottom
					? 'stickBottom'
					: trimmed.trimmedTop
						? 'preserve'
						: 'none'

				updateLogs(trimmed.logs, mode)
				if (trimmed.trimmedTop) {
					setHasOlder(true)
				}
			},
			{ defer: true },
		),
	)

	return (
		<div
			class={styles.container}
			ref={(el) => (containerElem = el)}
			onScroll={onScroll}
		>
			<div class={styles.scroller}>
				<Show when={isLoadingTop()}>
					<div class={styles.marker}>Loading older logs...</div>
				</Show>
				<Show when={!hasOlder() && logs().length > 0}>
					<div class={styles.marker}>Start of logs</div>
				</Show>
				<Show when={logs().length === 0}>
					<div class={styles.marker}>No logs yet</div>
				</Show>
				<For each={logs()}>
					{(msg) => <LogMsg msg={msg} formatAttrs={formatAttrs} />}
				</For>
				<Show when={isLoadingBottom()}>
					<div class={styles.marker}>Loading newer logs...</div>
				</Show>
			</div>
		</div>
	)
}
