/* @refresh reload */
import { render } from 'solid-js/web'
import './styles.css'

import { Loader } from './Loader'

const root = document.getElementById('root')

render(() => <Loader />, root!)
