import { Component, createSignal, For, onMount, Show } from 'solid-js'

import stylesCommon from '../common.module.css'
import { ConnectError } from '@connectrpc/connect'
import { useRpcClient } from '../ctx'

const P2pSettings: Component = () => {
	const client = useRpcClient()

	const [isLoading, setLoading] = createSignal(false)

	const [pendingAddr, setPendingAddr] = createSignal('')

	const [disable, setDisable] = createSignal(false)
	const [addrs, setAddrs] = createSignal<string[]>([])
	const [defaultPort, setDefaultPort] = createSignal(20048)
	const [disableProbe, setDisableProbe] = createSignal(false)
	const [adPrivate, setAdPrivate] = createSignal(false)
	const [disablePublicIpDiscovery, setDisablePublicIpDiscovery] =
		createSignal(false)
	const [disableUpnp, setDisableUpnp] = createSignal(false)
	const [upnpTimeoutMs, setUpnpTimeoutMs] = createSignal(10_000)

	const [error, setError] = createSignal('')
	const [isSaving, setSaving] = createSignal(false)
	const [isSuccess, setSuccess] = createSignal(false)
	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

		if (isSaving()) {
			return
		}

		setError('')
		setSuccess(false)
		setSaving(true)

		try {
			await client.updateDirectSettings({
				settings: {
					disable: disable(),
					addresses: addrs(),
					defaultPort: defaultPort(),
					disableProbeIpsToAdvertise: disableProbe(),
					advertisePrivateIps: adPrivate(),
					disablePublicIpDiscovery: disablePublicIpDiscovery(),
					disableUpnp: disableUpnp(),
					upnpTimeoutMs: upnpTimeoutMs(),
				},
			})

			setSuccess(true)
		} catch (err) {
			if (err instanceof ConnectError) {
				setError(err.message)
			} else {
				console.error('failed to update server:', err)
				setError('Internal error, check console')
			}
		} finally {
			setSaving(false)
		}
	}

	onMount(async () => {
		setLoading(true)
		try {
			const res = await client.getDirectSettings({})
			const cfg = res.settings!

			setDisable(cfg.disable)
			setAddrs(cfg.addresses)
			setDefaultPort(cfg.defaultPort)
			setDisableProbe(cfg.disableProbeIpsToAdvertise)
			setAdPrivate(cfg.advertisePrivateIps)
			setDisablePublicIpDiscovery(cfg.disablePublicIpDiscovery)
			setDisableUpnp(cfg.disableUpnp)
			setUpnpTimeoutMs(cfg.upnpTimeoutMs)
		} catch (err) {
			console.error('failed to get direct conncetion settings:', err)
			setError('Internal error, check console')
		} finally {
			setLoading(false)
		}
	})

	return (
		<div>
			<h2>P2P Settings</h2>

			<p>
				These settings control how the client connects to other peers
				and how other peers connect to it.
			</p>
			<p>
				Click the Stop Client button to stop the client, then start it
				again for changes to take effect.
			</p>

			<br />

			<Show when={error()}>
				<div class={stylesCommon.errorMessage}>{error()}</div>
			</Show>
			<Show when={isSuccess()}>
				<div class={stylesCommon.successMessage}>
					Settings Saved.
					<br />
					Restart your client for changes to take effect.
				</div>
			</Show>

			<Show
				when={isLoading()}
				fallback={
					<form onSubmit={submit} class={stylesCommon.form}>
						<table>
							<tbody>
								<tr>
									<td>
										<label for="setting-disable">
											Disable direct connections?
										</label>
									</td>
									<td>
										<input
											id="setting-disable"
											type="checkbox"
											placeholder=""
											onChange={(e) =>
												setDisable(
													e.currentTarget.checked,
												)
											}
											checked={disable()}
										/>
									</td>
								</tr>

								<Show when={!disable()}>
									<tr>
										<td>
											<label for="setting-addresses">
												Manually listen on these
												addresses:
											</label>
										</td>
										<td>
											<For each={addrs()}>
												{(addr) => (
													<div>
														<code>{addr}</code>{' '}
														<button
															type="button"
															onClick={() => {
																setAddrs(
																	addrs().filter(
																		(a) =>
																			a !==
																			addr,
																	),
																)
															}}
														>
															x
														</button>
													</div>
												)}
											</For>

											<br />

											<input
												type="text"
												placeholder="ex: 0.0.0.0:20048, [::]:20048"
												value={pendingAddr()}
												onInput={(e) =>
													setPendingAddr(
														e.currentTarget.value,
													)
												}
												onKeyDown={(e) => {
													if (e.key === 'Enter') {
														e.preventDefault()
													}
												}}
											/>
											<button
												type="button"
												onClick={() => {
													const addr = pendingAddr()
													if (!addr) {
														return
													}
													const exists = addrs().some(
														(a) => a === addr,
													)
													if (exists) {
														return
													}

													setAddrs([...addrs(), addr])
													setPendingAddr('')
												}}
											>
												Add
											</button>
										</td>
									</tr>

									<tr>
										<td>
											<label for="setting-default-port">
												Default port, or 0 for random:
											</label>
										</td>
										<td>
											<input
												type="number"
												id="setting-default-port"
												min={0}
												max={65535}
												value={defaultPort()}
												onInput={(e) =>
													setDefaultPort(
														parseInt(
															e.currentTarget
																.value,
														),
													)
												}
											/>
										</td>
									</tr>

									<tr>
										<td>
											<label for="setting-disable-probe">
												Disable probing the machine's
												interfaces for IPs to advertise?
											</label>
										</td>
										<td>
											<input
												type="checkbox"
												id="setting-disable-probe"
												placeholder=""
												onChange={(e) =>
													setDisableProbe(
														e.currentTarget.checked,
													)
												}
												checked={disableProbe()}
											/>
										</td>
									</tr>

									<tr>
										<td>
											<label for="setting-advertise-private">
												Advertise private IPs?
											</label>
										</td>
										<td>
											<input
												type="checkbox"
												id="setting-advertise-private"
												placeholder=""
												onChange={(e) =>
													setAdPrivate(
														e.currentTarget.checked,
													)
												}
												checked={adPrivate()}
											/>
										</td>
									</tr>

									<tr>
										<td>
											<label for="setting-disable-public-ip-discovery">
												Disable public IP discovery? If
												checked, the client will not
												query servers for the client's
												public IP.
											</label>
										</td>
										<td>
											<input
												type="checkbox"
												id="setting-disable-public-ip-discovery"
												placeholder=""
												onChange={(e) =>
													setDisablePublicIpDiscovery(
														e.currentTarget.checked,
													)
												}
												checked={disablePublicIpDiscovery()}
											/>
										</td>
									</tr>

									<tr>
										<td>
											<label for="setting-disable-upnp">
												Disable UPnP?
											</label>
										</td>
										<td>
											<input
												type="checkbox"
												id="setting-disable-upnp"
												placeholder=""
												onChange={(e) =>
													setDisableUpnp(
														e.currentTarget.checked,
													)
												}
												checked={disableUpnp()}
											/>
										</td>
									</tr>

									<tr>
										<td>
											<label for="setting-upnp-timeout-ms">
												UPnP timeout (milliseconds)
											</label>
										</td>
										<td>
											<input
												type="number"
												id="setting-upnp-timeout-ms"
												placeholder=""
												onChange={(e) =>
													setUpnpTimeoutMs(
														parseInt(
															e.currentTarget
																.value,
														),
													)
												}
												value={upnpTimeoutMs()}
											/>
										</td>
									</tr>
								</Show>
							</tbody>
						</table>

						<input
							type="submit"
							value="Update Settings (Requires Restart)"
							disabled={isSaving()}
						/>
					</form>
				}
			>
				Loading settings...
			</Show>
		</div>
	)
}

export const SettingsPage: Component = () => {
	return (
		<div
			classList={{
				[stylesCommon.center]: true,
				[stylesCommon.w100]: true,
			}}
		>
			<h1>Client Settings</h1>

			<P2pSettings />
		</div>
	)
}
