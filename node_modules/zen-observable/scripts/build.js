const { execSync } = require('child_process');
const plugins = require('./babel-plugins');

execSync('babel src --out-dir lib --plugins=' + plugins.join(','), {
  env: process.env,
  stdio: 'inherit',
});
