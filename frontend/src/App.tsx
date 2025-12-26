import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { findMatch, getState, makeMove } from "./api";
import type { MyState, Player, State } from "./types";
import { playerLabel } from "./types";

function isPlayableBoard(localWinner: Player): boolean {
  return localWinner === 2; // only if LocalState.winner == None
}

function allowedBoardFromLocation(location: number): { bx: number; by: number } | null {
  if (location === -1) return null;
  return { bx: Math.floor(location / 3), by: location % 3 };
}

function cellBg(p: Player): string {
  if (p === 0) return "cell cell-x";
  if (p === 1) return "cell cell-o";
  return "cell";
}

export function App() {
  const [session, setSession] = useState<MyState | null>(null);
  const [state, setState] = useState<State | null>(null);
  const [status, setStatus] = useState<"idle" | "matching" | "playing">("idle");
  const [error, setError] = useState<string | null>(null);
  const [boardSizePx, setBoardSizePx] = useState<number>(0);

  const matchAbortRef = useRef<AbortController | null>(null);
  const latestStateRef = useRef<State | null>(null);
  const boardAreaRef = useRef<HTMLDivElement | null>(null);

  const myRole = session?.role ?? 2;
  const gameId = session?.id ?? null;

  const toMove = state?.to_move ?? 2;
  const itsMyTurn = status === "playing" && myRole !== 2 && toMove === myRole;

  const allowedBoard = useMemo(() => (state ? allowedBoardFromLocation(state.location) : null), [state]);

  function resetAll() {
    matchAbortRef.current?.abort();
    matchAbortRef.current = null;
    setSession(null);
    setState(null);
    setStatus("idle");
    setError(null);
  }

  async function onFindMatch() {
    resetAll();
    setStatus("matching");
    setError(null);

    const ac = new AbortController();
    matchAbortRef.current = ac;

    try {
      const ms = await findMatch(ac.signal);
      setSession(ms);
      setState(ms.game_state);
      setStatus("playing");
    } catch (e) {
      if (ac.signal.aborted) return;
      setStatus("idle");
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      matchAbortRef.current = null;
    }
  }

  async function onCancelMatch() {
    matchAbortRef.current?.abort();
    matchAbortRef.current = null;
    setStatus("idle");
  }

  // Ensure the full 3x3 board always fits on screen: compute a square size from available board area.
  useLayoutEffect(() => {
    if (status !== "playing") return;
    const el = boardAreaRef.current;
    if (!el) return;

    const measure = () => {
      const rect = el.getBoundingClientRect();
      // Safety margin so it never touches edges / causes scrollbars.
      const size = Math.floor(Math.max(0, Math.min(rect.width, rect.height) - 4));
      setBoardSizePx(size);
    };

    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    window.addEventListener("resize", measure);

    return () => {
      ro.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [status]);

  // Keep a ref to the latest state so our polling loop doesn't capture stale values.
  useEffect(() => {
    latestStateRef.current = state;
  }, [state]);

  // Poll for opponent moves (and keep us in sync). Uses a single loop to avoid request storms.
  useEffect(() => {
    if (status !== "playing" || !gameId) return;
    const id = gameId;
    const ac = new AbortController();
    let stopped = false;
    let timeoutId: number | null = null;

    async function pollOnce() {
      try {
        const ms = await getState(id, ac.signal);
        if (stopped) return;
        setSession(ms); // role/id same, but harmless
        setState(ms.game_state);
        setError(null);
      } catch (e) {
        if (ac.signal.aborted || stopped) return;
        setError(e instanceof Error ? e.message : String(e));
      }
    }

    function scheduleNext(delayMs: number) {
      timeoutId = window.setTimeout(async () => {
        if (stopped) return;

        const s = latestStateRef.current;
        // Only poll frequently when the game is ongoing and it's not our turn.
        const shouldPoll = !s || (s.winner === 2 && s.to_move !== myRole);
        if (shouldPoll) await pollOnce();
        scheduleNext(700);
      }, delayMs);
    }

    // initial sync, then steady polling
    void pollOnce();
    scheduleNext(700);

    return () => {
      stopped = true;
      ac.abort();
      if (timeoutId != null) window.clearTimeout(timeoutId);
    };
  }, [status, gameId, myRole]);

  async function onClickCell(bx: number, by: number, cx: number, cy: number) {
    if (!state || !session || !gameId) return;
    if (!itsMyTurn) return;
    if (state.winner !== 2) return;

    // Enforce allowed board: if location != -1, you must play in that board.
    if (allowedBoard && (allowedBoard.bx !== bx || allowedBoard.by !== by)) return;

    const local = state.values[bx][by];
    if (!isPlayableBoard(local.winner)) return;
    if (local.values[cx][cy] !== 2) return;

    setError(null);
    try {
      const next = await makeMove(gameId, {
        player: session.role,
        cellX: bx,
        cellY: by,
        finalX: cx,
        finalY: cy,
      });
      setState(next);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      // refresh (maybe opponent moved)
      try {
        const ms = await getState(gameId);
        setState(ms.game_state);
      } catch {
        // ignore
      }
    }
  }

  const headline = useMemo(() => {
    if (status === "matching") return "Finding match…";
    if (status === "idle") return "Ultimate Tic-Tac-Toe";
    return "In game";
  }, [status]);

  return (
    <div className="page" style={{ ["--boardSize" as any]: `${boardSizePx}px` }}>
      <div className="topbar">
        <div>
          <div className="title">{headline}</div>
          <div className="subtitle">
            {status === "playing" && state ? (
              <>
                You are <b>{playerLabel(myRole)}</b> • Turn: <b>{playerLabel(state.to_move)}</b>{" "}
                {state.winner !== 2 ? (
                  <>
                    • Winner: <b>{playerLabel(state.winner)}</b>
                  </>
                ) : null}
              </>
            ) : (
              <>Matchmaking pairs two players when both click “Find match”.</>
            )}
          </div>
        </div>
        <div className="actions">
          {status === "idle" ? (
            <button className="btn primary" onClick={onFindMatch}>
              Find match
            </button>
          ) : null}
          {status === "matching" ? (
            <button className="btn" onClick={onCancelMatch}>
              Cancel
            </button>
          ) : null}
          {status === "playing" ? (
            <button className="btn" onClick={resetAll}>
              New match
            </button>
          ) : null}
        </div>
      </div>

      {error ? <div className="error">Error: {error}</div> : null}

      {status === "playing" && state && session ? (
        <div className="boardWrap">
          <div className="meta">
            {itsMyTurn ? <span className="pill ok">Your move</span> : <span className="pill">Waiting…</span>}
            {allowedBoard ? (
              <span className="pill">
                Must play in board ({allowedBoard.bx + 1},{allowedBoard.by + 1})
              </span>
            ) : (
              <span className="pill">Any board</span>
            )}
          </div>

          <div className="boardArea" ref={boardAreaRef}>
            <div className="board">
              {state.values.map((row, bx) =>
                row.map((local, by) => {
                  const forced = allowedBoard ? allowedBoard.bx === bx && allowedBoard.by === by : true;
                  const playable = isPlayableBoard(local.winner);
                  const boardClass =
                    "localBoard" +
                    (forced ? " localBoard-forced" : "") +
                    (playable ? "" : " localBoard-locked");
                  return (
                    <div key={`${bx}-${by}`} className={boardClass}>
                      <div className="localTop">
                        <div className="localLabel">
                          {bx + 1},{by + 1}
                        </div>
                        <div className="localWinner">
                          {local.winner !== 2 ? `Winner: ${playerLabel(local.winner)}` : ""}
                        </div>
                      </div>
                      <div className="localGrid">
                        {local.values.map((cellsRow, cx) =>
                          cellsRow.map((cell, cy) => (
                            <button
                              key={`${bx}-${by}-${cx}-${cy}`}
                              className={cellBg(cell)}
                              onClick={() => void onClickCell(bx, by, cx, cy)}
                              disabled={!itsMyTurn || cell !== 2 || !playable || (!!allowedBoard && !forced)}
                              title={`Board ${bx + 1},${by + 1} • Cell ${cx + 1},${cy + 1}`}
                            >
                              {playerLabel(cell)}
                            </button>
                          )),
                        )}
                      </div>
                    </div>
                  );
                }),
              )}
            </div>
          </div>

          <div className="footer">
            <div className="muted">
              Game id: <code>{String(session.id)}</code>
            </div>
          </div>
        </div>
      ) : (
        <div className="empty">
          <div className="card">
            <div className="cardTitle">How to play</div>
            <div className="cardBody">
              <ol>
                <li>Click <b>Find match</b> in two browser windows (or with a friend).</li>
                <li>Play in the highlighted mini-board (or anywhere if “Any board”).</li>
                <li>Your move sends the opponent to the mini-board matching your chosen cell.</li>
              </ol>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}


