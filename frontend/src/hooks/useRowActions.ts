import { useState } from "react";

import { useToast } from "../components/Toasts";

/**
 * The four things every inline row action needs: run it, say what happened, say
 * what was refused, and stop the user pressing it twice.
 *
 * The refusal half is why this exists rather than a bare `await`. Every
 * master-data row can be refused by the server for a reason the user has to read
 * — `in_use` on a warehouse holding stock (G5), on a supplier with open orders
 * (G4), on a code somebody else has taken since. The envelope's sentence is
 * written to be shown, and dropping it on the floor turns a delete that was
 * correctly refused into a button that did nothing.
 */
export function useRowActions(onChanged: () => void) {
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  async function run(action: () => Promise<unknown>, confirmation: string) {
    setBusy(true);
    try {
      await action();
      toast.success(confirmation);
      onChanged();
      return true;
    } catch (caught) {
      toast.failure(caught);
      return false;
    } finally {
      setBusy(false);
    }
  }

  return { busy, run };
}
