// Fills a Situation lobby with bots that stay connected and play every phase
// on their own, so a single person can test a full game from the browser.
//
//   corepack pnpm bots:situation <CODE> [--bots 3] [--api http://localhost:8080]
//
// The human stays host: they create the lobby, start the game, flip through
// the reveal and launch the next rounds. Bots never take host actions.

const botNames = ['Bot Camille', 'Bot Sacha', 'Bot Léo', 'Bot Jade', 'Bot Noé', 'Bot Inès', 'Bot Malo', 'Bot Lou', 'Bot Eden'];

const botThemes = [
  'Seul·e au volant d’un bus scolaire en panne',
  'Témoin principal dans un procès de pigeons',
  'Coincé·e dans un escape game depuis 3 jours',
  'Chef·fe cuisinier·e pour un dîner de vampires',
  'Guide touristique sur Mars',
  'Candidat·e surprise à Koh-Lanta',
  'Maître·sse nageur·se dans une piscine de gelée',
];

const botReasons = [
  'Il/elle garde son calme en toutes circonstances',
  'Aucune limite, aucune honte',
  'A déjà survécu à pire, crois-moi',
  'Le charisme d’un gourou de secte',
  'Ment avec un aplomb inquiétant',
  'Ferait copain-copain avec tout le monde en 5 min',
  'Trouverait un plan foireux mais qui marche',
  'Pleurerait, puis gagnerait quand même',
];

function parseArguments(argv) {
  const options = { code: '', bots: 3, api: 'http://localhost:8080' };
  for (let index = 0; index < argv.length; index++) {
    const argument = argv[index];
    if (argument === '--bots') {
      options.bots = Number(argv[++index]);
    } else if (argument === '--api') {
      options.api = argv[++index].replace(/\/$/, '');
    } else if (!options.code) {
      options.code = argument.toUpperCase();
    }
  }
  if (!options.code || !Number.isInteger(options.bots) || options.bots < 1 || options.bots > botNames.length) {
    process.stderr.write(
      `Usage : corepack pnpm bots:situation <CODE> [--bots 1-${botNames.length}] [--api http://localhost:8080]\n`,
    );
    process.exit(1);
  }
  return options;
}

const options = parseArguments(process.argv.slice(2));
const websocketBase = options.api.replace(/^http/, 'ws');

function pick(list) {
  return list[Math.floor(Math.random() * list.length)];
}

function shuffle(list) {
  const copy = [...list];
  for (let index = copy.length - 1; index > 0; index--) {
    const other = Math.floor(Math.random() * (index + 1));
    [copy[index], copy[other]] = [copy[other], copy[index]];
  }
  return copy;
}

function thinkingDelay() {
  return new Promise((resolve) => setTimeout(resolve, 800 + Math.random() * 1700));
}

class SituationBot {
  constructor(displayName) {
    this.displayName = displayName;
    this.reconnectToken = '';
    this.state = null;
    this.isActing = false;
    this.stopped = false;
  }

  log(message) {
    process.stdout.write(`[${this.displayName}] ${message}\n`);
  }

  async request(path, body) {
    const response = await fetch(`${options.api}/api/v1/lobbies/${options.code}${path}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(this.reconnectToken ? { Authorization: `Bearer ${this.reconnectToken}` } : {}),
      },
      body: JSON.stringify(body),
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const error = payload.error ?? payload;
      throw new Error(`${response.status} ${error.code ?? ''} ${error.message ?? ''}`.trim());
    }
    return payload;
  }

  async join() {
    const session = await this.request('/players', { displayName: this.displayName });
    this.reconnectToken = session.reconnectToken;
    this.state = session.state;
    this.log('a rejoint le lobby');
    this.connect();
  }

  connect() {
    const socket = new WebSocket(`${websocketBase}/ws/lobbies/${options.code}`);
    socket.addEventListener('open', () => {
      socket.send(JSON.stringify({ type: 'authenticate', reconnectToken: this.reconnectToken }));
    });
    socket.addEventListener('message', (event) => {
      const message = JSON.parse(event.data);
      if (message.type === 'state.snapshot') {
        this.state = message.state;
        void this.act();
      }
    });
    socket.addEventListener('close', (event) => {
      if (this.stopped) {
        return;
      }
      // 4401 (invalid session) and 4404 (lobby gone) are final, like in the web client.
      if (event.code === 4401 || event.code === 4404) {
        this.log(`déconnecté pour de bon (${event.code} ${event.reason})`);
        this.stopped = true;
        return;
      }
      setTimeout(() => this.connect(), 1_000);
    });
    this.socket = socket;
  }

  stop() {
    this.stopped = true;
    this.socket?.close();
  }

  // Snapshots keep arriving while a bot "thinks": only one action runs at a
  // time, and the latest snapshot is re-evaluated once it finishes.
  async act() {
    if (this.isActing || this.stopped) {
      return;
    }
    this.isActing = true;
    try {
      while (await this.takeNextAction()) {
        // Keep going until the current snapshot asks nothing more of this bot.
      }
    } catch (error) {
      this.log(`erreur : ${error.message}`);
    } finally {
      this.isActing = false;
    }
  }

  async takeNextAction() {
    const game = this.state?.situationGame;
    if (!game) {
      return false;
    }
    const action = this.nextAction(game);
    if (!action) {
      return false;
    }
    await thinkingDelay();
    // The snapshot may have moved on while thinking (deadline, other votes…).
    if (this.nextAction(this.state?.situationGame)?.key !== action.key) {
      return true;
    }
    const response = await action.run();
    if (response?.situationGame) {
      this.state = response;
    }
    return true;
  }

  nextAction(game) {
    if (!game) {
      return null;
    }
    if (game.phase === 'collecting_themes' && !game.themeSubmitted) {
      return {
        key: `${game.id}:theme`,
        run: () => {
          const theme = pick(botThemes);
          this.log(`propose la situation « ${theme} »`);
          return this.request('/situation/themes', { theme });
        },
      };
    }
    if (game.phase === 'ranking_themes' && !game.themeRanked && game.themeCandidates?.length) {
      return {
        key: `${game.id}:rank-themes`,
        run: () => {
          this.log('classe les situations');
          return this.request('/situation/themes/rank', { ranking: shuffle(game.themeCandidates) });
        },
      };
    }
    if (game.phase !== 'playing') {
      return null;
    }

    const roundKey = `${game.id}:${game.roundNumber}`;
    if (game.roundPhase === 'proposing' && !game.submitted) {
      return {
        key: `${roundKey}:proposal`,
        run: () => {
          const others = this.state.players.filter((player) => player.id !== this.state.currentPlayerId);
          const chosen = pick(others);
          const reason = pick(botReasons);
          this.log(`choisit ${chosen.displayName} : « ${reason} »`);
          return this.request('/situation/proposal', { chosenPlayerId: chosen.id, reason });
        },
      };
    }
    if (game.roundPhase === 'dueling' && game.currentDuel) {
      return this.duelAction(game.currentDuel);
    }
    if (game.roundPhase === 'ranking' && !game.rankingSubmitted && game.rankingCandidates?.length) {
      return {
        key: `${roundKey}:ranking`,
        run: () => {
          this.log('classe les finalistes');
          const ranking = shuffle(game.rankingCandidates.map((proposal) => proposal.id));
          return this.request('/situation/ranking', { ranking });
        },
      };
    }
    return null;
  }

  // Bots are agreeable: they all open on the same proposal (lowest id) so two
  // bots agree at once, and if the opponent has voted while the duel is still
  // open, the votes differ — the bot then rallies to the other proposal.
  duelAction(duel) {
    const [first, second] = [duel.proposalA.id, duel.proposalB.id].sort();
    let target = null;
    if (!duel.myVoteProposalId) {
      target = first;
    } else if (duel.opponentHasVoted) {
      target = duel.myVoteProposalId === first ? second : first;
    }
    if (!target) {
      return null;
    }
    const chosen = target === duel.proposalA.id ? duel.proposalA : duel.proposalB;
    return {
      key: `${duel.id}:${duel.myVoteProposalId ?? ''}:${target}`,
      run: () => {
        this.log(`duel contre ${duel.opponentDisplayName} : vote pour « ${chosen.chosenDisplayName} — ${chosen.reason} »`);
        return this.request(`/situation/duels/${duel.id}/vote`, { proposalId: target });
      },
    };
  }
}

const bots = botNames.slice(0, options.bots).map((name) => new SituationBot(name));

process.stdout.write(`\n[bots] ${bots.length} bot(s) rejoignent le lobby ${options.code} via ${options.api}\n\n`);
let joinedCount = 0;
for (const bot of bots) {
  try {
    await bot.join();
    joinedCount++;
  } catch (error) {
    bot.log(`impossible de rejoindre : ${error.message}`);
  }
}
if (joinedCount === 0) {
  process.stderr.write(`\n[bots] Aucun bot n’a pu rejoindre. L’API tourne-t-elle sur ${options.api} et le code est-il bon ?\n`);
  process.exit(1);
}
process.stdout.write('\n[bots] Prêts. Lance la partie depuis ton navigateur (tu es l’hôte). Ctrl+C pour arrêter.\n\n');

process.on('SIGINT', () => {
  for (const bot of bots) {
    bot.stop();
  }
  process.exit(0);
});
