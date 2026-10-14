// Allow jest import static assets

/* eslint-disable-next-line @typescript-eslint/no-require-imports */
const path = require('path');

module.exports = {
  process(_, filename) {
    return 'module.exports = ' + JSON.stringify(path.basename(filename)) + ';';
  },
};
