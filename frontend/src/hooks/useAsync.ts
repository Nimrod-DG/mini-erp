import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Every list and detail screen needs default, loading, empty, and error states
 * (§10.7.6), and none of them are optional. Modelling the four as one union
 * rather than three loose booleans is what makes forgetting the error case a
 * type error instead of a blank screen.
 */
export type AsyncState<T> =
  | { status: "loading" }
  | { status: "error"; error: Error }
  | { status: "ready"; data: T };

/**
 * Runs `load` when `key` changes, and again on `reload()`.
 *
 * The fetcher is held in a ref and the effect keyed on a string, so callers do
 * not have to memoise a closure to avoid a refetch loop — the most common way
 * this pattern goes wrong.
 *
 * A resolution from a superseded request is discarded. Typing in a search box
 * fires several of these, and they do not come back in order; without the
 * generation counter the screen settles on whichever was slowest.
 */
export function useAsync<T>(key: string, load: () => Promise<T>) {
  const [state, setState] = useState<AsyncState<T>>({ status: "loading" });
  const [nonce, setNonce] = useState(0);

  const loadRef = useRef(load);
  loadRef.current = load;

  const generation = useRef(0);

  useEffect(() => {
    const current = ++generation.current;
    setState({ status: "loading" });

    loadRef.current().then(
      (data) => {
        if (generation.current === current) setState({ status: "ready", data });
      },
      (error: unknown) => {
        if (generation.current === current) {
          setState({
            status: "error",
            error: error instanceof Error ? error : new Error(String(error)),
          });
        }
      },
    );
  }, [key, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  return { state, reload };
}
