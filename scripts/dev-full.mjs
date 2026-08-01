import { spawn, spawnSync } from 'node:child_process';
import { copyFileSync, existsSync } from 'node:fs';
import { createServer } from 'node:net';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const projectRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const webRoot = join(projectRoot, 'apps', 'web');
const angularCliPath = join(webRoot, 'node_modules', '@angular', 'cli', 'bin', 'ng.js');
const dockerCommand = process.platform === 'win32' ? 'docker.exe' : 'docker';
const apiReadyUrl = 'http://localhost:8080/api/v1';
const angularPort = 4200;
let angularProcess;
let composeStarted = false;
let isShuttingDown = false;

process.chdir(projectRoot);

function printStep(message) {
  process.stdout.write(`\n[dev:full] ${message}\n`);
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: projectRoot,
    stdio: 'inherit',
    ...options,
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(' ')} failed with exit code ${result.status}`);
  }
}

async function assertPortAvailable(port) {
  await new Promise((resolve, reject) => {
    const server = createServer();
    server.unref();
    server.once('error', (error) => {
      if (error.code === 'EADDRINUSE') {
        reject(new Error(`Le port ${port} est déjà utilisé. Arrête l'ancien serveur avec Ctrl+C.`));
        return;
      }
      reject(error);
    });
    server.listen({ port, exclusive: true }, () => {
      server.close(resolve);
    });
  });
}

async function waitForApi() {
  const deadline = Date.now() + 120_000;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(apiReadyUrl, {
        signal: AbortSignal.timeout(2_000),
      });
      if (response.ok) {
        return;
      }
    } catch {
      // Docker may still be starting PostgreSQL, migrations, the API or Nginx.
    }
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  throw new Error(`L'API n'est pas devenue disponible sur ${apiReadyUrl} après 120 secondes.`);
}

function stopAngularProcess() {
  if (!angularProcess || angularProcess.exitCode !== null || !angularProcess.pid) {
    return;
  }
  if (process.platform === 'win32') {
    spawnSync('taskkill.exe', ['/PID', String(angularProcess.pid), '/T', '/F'], {
      stdio: 'ignore',
    });
    return;
  }
  angularProcess.kill('SIGTERM');
}

function stopCompose() {
  if (!composeStarted) {
    return;
  }
  printStep('Arrêt de la stack Docker (les volumes de données sont conservés)…');
  spawnSync(dockerCommand, ['compose', 'down', '--remove-orphans'], {
    cwd: projectRoot,
    stdio: 'inherit',
  });
  composeStarted = false;
}

function shutdown(exitCode = 0) {
  if (isShuttingDown) {
    return;
  }
  isShuttingDown = true;
  stopAngularProcess();
  stopCompose();
  process.exit(exitCode);
}

process.on('SIGINT', () => shutdown(0));
process.on('SIGTERM', () => shutdown(0));

async function main() {
  printStep('Vérification du port Angular…');
  await assertPortAvailable(angularPort);

  const envPath = join(projectRoot, '.env');
  if (!existsSync(envPath)) {
    copyFileSync(join(projectRoot, '.env.example'), envPath);
    printStep('Fichier .env créé depuis .env.example pour le développement local.');
  }

  printStep('Vérification de Docker Desktop…');
  const dockerInfo = spawnSync(dockerCommand, ['info'], {
    cwd: projectRoot,
    stdio: 'ignore',
  });
  if (dockerInfo.error || dockerInfo.status !== 0) {
    throw new Error('Docker Desktop ne répond pas. Démarre-le puis relance cette commande.');
  }

  printStep('Build et démarrage de PostgreSQL, des migrations, de l’API et du proxy web…');
  composeStarted = true;
  run(dockerCommand, ['compose', 'up', '-d', '--build', '--remove-orphans']);

  printStep('Attente de l’API…');
  await waitForApi();

  printStep(`API prête. Démarrage d’Angular sur http://localhost:${angularPort}/`);
  printStep('Utilise Ctrl+C pour arrêter Angular et Docker proprement.');
  if (!existsSync(angularCliPath)) {
    throw new Error('Angular CLI est absent. Exécute « corepack pnpm install » puis relance la commande.');
  }
  angularProcess = spawn(
    process.execPath,
    [
      angularCliPath,
      'serve',
      '--proxy-config',
      'proxy.conf.json',
      '--port',
      String(angularPort),
    ],
    {
      cwd: webRoot,
      stdio: 'inherit',
      windowsHide: false,
    },
  );
  angularProcess.once('error', (error) => {
    console.error(`\n[dev:full] Impossible de démarrer Angular : ${error.message}`);
    shutdown(1);
  });
  angularProcess.once('exit', (code) => {
    shutdown(code ?? 0);
  });
}

main().catch((error) => {
  console.error(`\n[dev:full] ${error.message}`);
  stopCompose();
  process.exit(1);
});
