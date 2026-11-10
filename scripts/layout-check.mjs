// Checks that the screens reachable without a game fit the phone viewport, using the local
// Chrome in headless mode and the production build (no API: pages show their offline states).
//
//   corepack pnpm --filter @crush-club/web build
//   corepack pnpm layout:check            (CHROME_PATH=… if Chrome is not found)
//
// At 100 % text size every screen must fit without scrolling. At 125 % (accessibility setting)
// the body may scroll, but nothing may overflow sideways and the footer must stay on screen.

import { spawn } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { readFile, stat } from 'node:fs/promises';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { dirname, extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

const projectRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const buildRoot = join(projectRoot, 'apps', 'web', 'dist', 'browser');
const serverPort = 4310;
const debuggingPort = 9333;
const routeList = ['/', '/join', '/join?mode=create', '/game-configs', '/game-configs/new'];
// Chrome emulation has no browser toolbars: 375 × 553 is an iPhone SE in Safari, toolbars shown.
const viewportList = [
  [375, 667],
  [390, 664],
  [360, 640],
  [375, 553],
  [320, 568],
];
const enlargedTextViewport = [375, 553];
const contentTypeByExtension = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.webp': 'image/webp',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.webmanifest': 'application/manifest+json',
};

function findChrome() {
  const candidateList = [
    process.env.CHROME_PATH,
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
  ];
  const chromePath = candidateList.find((candidate) => candidate && existsSync(candidate));
  if (!chromePath) {
    throw new Error('Chrome est introuvable : définis CHROME_PATH.');
  }
  return chromePath;
}

function startStaticServer() {
  const server = createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://localhost');
    if (url.pathname.startsWith('/api/')) {
      response.writeHead(503, { 'content-type': 'application/json' });
      response.end('{"error":{"code":"unavailable","message":"API absente pendant le contrôle"}}');
      return;
    }
    let filePath = normalize(join(buildRoot, decodeURIComponent(url.pathname)));
    try {
      if (!filePath.startsWith(buildRoot) || !(await stat(filePath)).isFile()) {
        throw new Error('not a file');
      }
    } catch {
      filePath = join(buildRoot, 'index.html');
    }
    response.writeHead(200, { 'content-type': contentTypeByExtension[extname(filePath)] ?? 'application/octet-stream' });
    response.end(await readFile(filePath));
  });
  return new Promise((resolve) => server.listen(serverPort, () => resolve(server)));
}

async function connectToChrome() {
  for (let attempt = 0; attempt < 40; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 250));
    try {
      const targetList = await (await fetch(`http://127.0.0.1:${debuggingPort}/json`)).json();
      const page = targetList.find((target) => target.type === 'page');
      if (page) {
        const socket = new WebSocket(page.webSocketDebuggerUrl);
        await new Promise((resolve) => socket.addEventListener('open', resolve));
        return socket;
      }
    } catch {
      // Chrome is still starting.
    }
  }
  throw new Error('Chrome ne répond pas sur le port de débogage.');
}

function createCommandSender(socket) {
  let nextId = 0;
  const pendingById = new Map();
  socket.addEventListener('message', (event) => {
    const message = JSON.parse(event.data);
    pendingById.get(message.id)?.(message);
    pendingById.delete(message.id);
  });
  return (method, params = {}) =>
    new Promise((resolve) => {
      const id = ++nextId;
      pendingById.set(id, resolve);
      socket.send(JSON.stringify({ id, method, params }));
    });
}

// Runs in the page: measures the visible Ionic page against the viewport.
const measureExpression = `(() => {
  const page = [...document.querySelectorAll('.ion-page')]
    .filter((candidate) => !candidate.classList.contains('ion-page-hidden') && candidate.getBoundingClientRect().height > 0)
    .at(-1);
  if (!page) return { error: 'aucune page affichée' };
  const body = page.querySelector('.screen-body');
  const footer = page.querySelector(':scope > .screen-footer');
  // Content cut on the side: an in-flow element past the body's edge that no inner container clips or scrolls.
  let sideOverflow = document.documentElement.scrollWidth - innerWidth;
  if (body) {
    const bodyRight = body.getBoundingClientRect().right;
    for (const element of body.querySelectorAll('*')) {
      let ancestor = element;
      let isHandled = false;
      while (ancestor && ancestor !== body) {
        const style = getComputedStyle(ancestor);
        if (style.position === 'absolute' || style.position === 'fixed' || (ancestor !== element && style.overflowX !== 'visible')) {
          isHandled = true;
          break;
        }
        ancestor = ancestor.parentElement;
      }
      if (!isHandled) {
        sideOverflow = Math.max(sideOverflow, Math.round(element.getBoundingClientRect().right - bodyRight));
      }
    }
  }
  return {
    bodyOverflow: body ? body.scrollHeight - body.clientHeight : 0,
    sideOverflow,
    footerBottom: footer ? Math.round(footer.getBoundingClientRect().bottom) : 0,
    viewportHeight: innerHeight,
  };
})()`;

async function measure(send, route, [width, height], fontSize) {
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 2, mobile: true });
  await send('Page.navigate', { url: `http://localhost:${serverPort}${route}` });
  await new Promise((resolve) => setTimeout(resolve, 2_000));
  if (fontSize) {
    await send('Runtime.evaluate', { expression: `document.documentElement.style.fontSize = '${fontSize}'` });
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  const { result } = await send('Runtime.evaluate', { expression: measureExpression, returnByValue: true });
  return result.result.value;
}

async function main() {
  if (!existsSync(join(buildRoot, 'index.html'))) {
    throw new Error('Build absent : lance d’abord corepack pnpm --filter @crush-club/web build.');
  }
  const server = await startStaticServer();
  const profileDirectory = mkdtempSync(join(tmpdir(), 'crush-club-layout-'));
  const chrome = spawn(findChrome(), [
    '--headless=new',
    `--remote-debugging-port=${debuggingPort}`,
    `--user-data-dir=${profileDirectory}`,
    '--no-first-run',
    '--hide-scrollbars',
    'about:blank',
  ]);
  const failureList = [];
  try {
    const send = createCommandSender(await connectToChrome());
    for (const route of routeList) {
      for (const viewport of viewportList) {
        const result = await measure(send, route, viewport);
        const label = `${route} ${viewport.join('×')}`;
        if (result.error) failureList.push(`${label} : ${result.error}`);
        else if (result.bodyOverflow > 1) failureList.push(`${label} : dépasse de ${result.bodyOverflow}px`);
        else if (result.sideOverflow > 0) failureList.push(`${label} : déborde de ${result.sideOverflow}px sur le côté`);
        console.log(`${result.error || result.bodyOverflow > 1 || result.sideOverflow > 0 ? '✗' : '✓'} ${label}`);
      }
      const result = await measure(send, route, enlargedTextViewport, '125%');
      const label = `${route} ${enlargedTextViewport.join('×')} texte 125 %`;
      if (result.error) failureList.push(`${label} : ${result.error}`);
      else if (result.sideOverflow > 0) failureList.push(`${label} : déborde de ${result.sideOverflow}px sur le côté`);
      else if (result.footerBottom > result.viewportHeight) failureList.push(`${label} : footer hors de l’écran`);
      console.log(`${failureList.at(-1)?.startsWith(label) ? '✗' : '✓'} ${label}`);
    }
  } finally {
    const chromeExit = new Promise((resolve) => chrome.once('exit', resolve));
    chrome.kill();
    await Promise.race([chromeExit, new Promise((resolve) => setTimeout(resolve, 3_000))]);
    server.close();
    try {
      rmSync(profileDirectory, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
    } catch {
      // Windows may keep the profile locked a little longer; the temp folder is cleaned up later anyway.
    }
  }
  if (failureList.length > 0) {
    console.error(`\n${failureList.length} écran(s) ne tiennent pas :\n- ${failureList.join('\n- ')}`);
    process.exit(1);
  }
  console.log('\nTous les écrans contrôlés tiennent dans le viewport.');
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
});
