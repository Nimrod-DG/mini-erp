import { useId, useState } from "react";

/** 20×20 stroke icons, inline so there is no icon dependency and no request.
 *  aria-hidden: the button's accessible name carries the meaning, and a
 *  screen reader announcing "eye" alongside it is noise. */
function EyeIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      width="20"
      height="20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function EyeOffIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      width="20"
      height="20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M10.7 6.2A10.5 10.5 0 0 1 12 6c6.4 0 10 6 10 6a16.4 16.4 0 0 1-3 3.6" />
      <path d="M6.5 7.1A16.1 16.1 0 0 0 2 12s3.6 6 10 6a10 10 0 0 0 4.2-.9" />
      <path d="M9.9 9.9a3 3 0 0 0 4.2 4.2" />
      <path d="M3 3l18 18" />
    </svg>
  );
}

type Props = {
  value: string;
  onChange: (next: string) => void;
  label?: string;
  /** Distinct per field: two password inputs in one form must not share a name,
   *  or a password manager cannot tell them apart. */
  name?: string;
  autoComplete?: string;
  disabled?: boolean;
};

/**
 * A password input with a reveal toggle.
 *
 * The toggle is `type="button"`. Inside a form, a button without that attribute
 * defaults to `submit` — so revealing the password would submit the form, which
 * is the classic version of this bug.
 *
 * Visibility is deliberately not persisted anywhere: it resets on every mount,
 * so a revealed password cannot survive a navigation onto a shared screen.
 */
export function PasswordField({
  value,
  onChange,
  label = "Password",
  name = "password",
  autoComplete = "current-password",
  disabled = false,
}: Props) {
  const [visible, setVisible] = useState(false);
  const id = useId();

  return (
    <div className="flex flex-col gap-1.5 text-sm">
      <label htmlFor={id} className="font-medium">
        {label}
      </label>
      <div className="relative">
        <input
          id={id}
          type={visible ? "text" : "password"}
          name={name}
          autoComplete={autoComplete}
          required
          disabled={disabled}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          // pr-12 keeps the text clear of the button rather than running under it.
          className="min-h-11 w-full rounded-md border border-hairline bg-surface pl-3 pr-12 text-base"
        />
        <button
          type="button"
          onClick={() => setVisible((v) => !v)}
          // The state lives in the name, not only in aria-pressed: "Show
          // password" / "Hide password" is unambiguous read on its own.
          aria-label={visible ? "Hide password" : "Show password"}
          aria-pressed={visible}
          // Full-height 44px-wide target (§10 hit areas, FE9).
          className="absolute inset-y-0 right-0 grid w-11 place-items-center rounded-r-md text-secondary hover:text-primary"
        >
          {visible ? <EyeOffIcon /> : <EyeIcon />}
        </button>
      </div>
    </div>
  );
}
