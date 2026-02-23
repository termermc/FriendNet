import styles from './LogsPage.module.css'

import {
	Component,
	For,
	Show,
	createEffect,
	createSignal,
	onMount,
} from 'solid-js'
import { useGlobalState } from '../ctx'
import { LogMessage } from '../../pb/clientrpc/v1/rpc_pb'
import { collect } from '../util'

const pageSize = 200
const scrollThresholdPx = 200
const autoScrollThresholdPx = 120

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

export const LogsPage: Component = () => {
	const state = useGlobalState()

	const [logs, setLogs] = createSignal<LogMessage[]>([])
	const [hasOlder, setHasOlder] = createSignal(true)
	const [hasNewer, setHasNewer] = createSignal(true)
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

	const isNearTop = () =>
		containerElem ? containerElem.scrollTop <= scrollThresholdPx : false

	const isNearBottom = (threshold = scrollThresholdPx) => {
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

	const seedNow = () => {
		const nowTs = BigInt(Date.now())
		const existingUids = new Set<string>()

		const older = collect(iterateOlder(nowTs, existingUids), pageSize)
		for (const msg of older) {
			existingUids.add(msg.uid)
		}
		older.reverse()

		const newer = collect(iterateNewer(nowTs, existingUids), pageSize)
		const combined = [...older, ...newer]
		setLogs(combined)
		setHasOlder(older.length === pageSize)
		setHasNewer(newer.length === pageSize)

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
		const batch = collect(iterateOlder(beforeTs, existingUids), pageSize)

		if (batch.length === 0) {
			setHasOlder(false)
			setIsLoadingTop(false)
			return
		}

		batch.reverse()
		const container = containerElem
		const prevScrollHeight = container?.scrollHeight ?? 0
		const prevScrollTop = container?.scrollTop ?? 0
		setLogs([...batch, ...current])

		requestAnimationFrame(() => {
			if (!containerElem) {
				return
			}
			const delta = containerElem.scrollHeight - prevScrollHeight
			containerElem.scrollTop = prevScrollTop + delta
		})

		if (batch.length < pageSize) {
			setHasOlder(false)
		}
		setIsLoadingTop(false)
	}

	const loadNewer = () => {
		if (isLoadingBottom() || !hasNewer()) {
			return
		}

		setIsLoadingBottom(true)

		const current = logs()
		const afterTs =
			current.length > 0
				? current[current.length - 1].createdTs
				: BigInt(Date.now())
		const existingUids = makeUidSet(current)
		const batch = collect(iterateNewer(afterTs, existingUids), pageSize)

		if (batch.length === 0) {
			setHasNewer(false)
			setIsLoadingBottom(false)
			return
		}

		const stickToBottom = isNearBottom(autoScrollThresholdPx)
		setLogs([...current, ...batch])

		if (stickToBottom) {
			requestAnimationFrame(() => scrollToBottom())
		}

		if (batch.length < pageSize) {
			setHasNewer(false)
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
		seedNow()
	})

	createEffect(() => {
		state.log.logCount()
		if (logs().length === 0) {
			seedNow()
		}
	})

	createEffect(() => {
		const latest = state.log.latestLog()
		if (!latest) {
			return
		}

		const current = logs()
		if (current.some((msg) => msg.uid === latest.uid)) {
			return
		}

		const stickToBottom = isNearBottom(autoScrollThresholdPx)
		setLogs([...current, latest])
		setHasNewer(true)

		if (stickToBottom) {
			requestAnimationFrame(() => scrollToBottom())
		}
	})

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
				<Show when={!hasNewer() && logs().length > 0}>
					<div class={styles.marker}>Latest log</div>
				</Show>
			</div>
		</div>
	)
}
