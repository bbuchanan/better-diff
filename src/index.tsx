import {resolve} from 'node:path';
import React from 'react';
import {render} from 'ink';

import {App} from './app/App.js';

const cwd = process.argv[2] ? resolve(process.argv[2]) : process.cwd();

render(<App cwd={cwd} />);
