import { BehaviorComponent, BehaviorLoader, html } from 'wunphile'
import type { Component } from 'wunphile'
import config from '../../../config.ts'

/**
 * The server config validator page.
 */
export const ServerConfigValidator: Component<void, void> = () => {
	// prettier-ignore
	return html`
		<!DOCTYPE html>
		<html lang="en">
		<head>
			<title>Server Config Validator</title>
		</head>
		<body style="padding: 0; margin: 0; height: 100vh; background: black; color: white;">
			${BehaviorComponent({ module: import('../../client/ServerConfigValidator.behavior.ts') }, html`
		        <pre id="editor-src">${html(JSON.stringify({
		            "$schema": config.prodRootUrl + config.serverConfigSchemaPath,
	                "listen": [
	                    "0.0.0.0:20038",
	                    "[::]:20038"
	                ],
	                "db_path": "server.db",
	                "pem_path": "server.pem",
	                "disable_update_checker": false,
	                "rpc": {
	                    "https_pem_path": "rpc.pem",
	                    "interfaces": [
	                        {
	                            "address": "unix://friendnet-server.sock",
	                            "allowed_methods": [
	                                "*"
	                            ],
	                            "cors_allow_all_origins": false,
	                            "enable_admin_ui": false
	                        },
	                        {
	                            "address": "http://127.0.0.1:8080",
	                            "allowed_methods": [
	                                "GetRooms",
	                                "GetRoomInfo",
	                                "GetOnlineUsers",
	                                "GetOnlineUserInfo"
	                            ],
	                            "cors_allow_all_origins": true,
	                            "enable_admin_ui": false
	                        }
	                    ]
	                }
		        }, null, 4))}</pre>
		        <div id="editor" style="display: none; width: 100%; height: 100%;"></div>
	        `)}
	
	        <script type="application/json" id="editor-config">${html(JSON.stringify({
	            prodRootUrl: config.prodRootUrl,
	            serverConfigSchemaPath: config.serverConfigSchemaPath,
	        }))}
	        </script>
	        <script>
	            window.require = {
	                paths: {
	                    vs: '/js/lib/monaco-editor/vs',
	                },
	            }
	        </script>
	        <script src="/js/lib/monaco-editor/vs/loader.js"></script>
			${BehaviorLoader()}
		</body>
		</html>
	`
}
