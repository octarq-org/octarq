// Pure ANSI color helpers and terminal layout utilities (Zero third-party dependencies)

export const colors = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  italic: '\x1b[3m',
  underline: '\x1b[4m',

  // Foreground colors
  black: '\x1b[30m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  white: '\x1b[37m',

  // Bright variants
  brightRed: '\x1b[91m',
  brightGreen: '\x1b[92m',
  brightYellow: '\x1b[93m',
  brightBlue: '\x1b[94m',
  brightMagenta: '\x1b[95m',
  brightCyan: '\x1b[96m',
  brightWhite: '\x1b[97m',

  // Background colors
  bgBlue: '\x1b[44m',
  bgMagenta: '\x1b[45m',
  bgCyan: '\x1b[46m',
  bgDark: '\x1b[48;5;236m',
};

export function banner(title, subtitle = '') {
  const line = '═'.repeat(66);
  console.log(`\n${colors.brightCyan}${line}${colors.reset}`);
  console.log(`${colors.bold}${colors.brightWhite}  🤖 OCTARQ BLUEPRINT: ${title}${colors.reset}`);
  if (subtitle) {
    console.log(`${colors.dim}     ${subtitle}${colors.reset}`);
  }
  console.log(`${colors.brightCyan}${line}${colors.reset}\n`);
}

export function section(stepNum, title, desc = '') {
  console.log(`\n${colors.bgBlue}${colors.brightWhite}${colors.bold} STEP ${stepNum} ${colors.reset} ${colors.bold}${colors.brightCyan}${title}${colors.reset}`);
  if (desc) {
    console.log(`${colors.dim}        ${desc}${colors.reset}`);
  }
  console.log(`${colors.dim}─`.repeat(66) + `${colors.reset}`);
}

export function success(message) {
  console.log(`${colors.brightGreen}✔ [SUCCESS]${colors.reset} ${message}`);
}

export function info(message) {
  console.log(`${colors.brightBlue}ℹ [INFO]${colors.reset}    ${message}`);
}

export function warn(message) {
  console.log(`${colors.brightYellow}⚠ [WARN]${colors.reset}    ${message}`);
}

export function error(message) {
  console.log(`${colors.brightRed}✖ [ERROR]${colors.reset}   ${message}`);
}

export function mcpCall(toolName, params) {
  console.log(`${colors.magenta}⚙ [MCP CALL]${colors.reset} ${colors.bold}${toolName}${colors.reset}(${JSON.stringify(params)})`);
}

export function mcpResult(toolName, output) {
  console.log(`${colors.brightGreen}◀ [MCP RETURN]${colors.reset} ${colors.bold}${toolName}${colors.reset}`);
  if (typeof output === 'object') {
    console.log(`${colors.dim}${JSON.stringify(output, null, 2)}${colors.reset}`);
  } else {
    console.log(`${colors.dim}${output}${colors.reset}`);
  }
}

export function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
