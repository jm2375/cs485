# Test specification: `InviteModal.tsx`

## Functions in this file

| # | Name | Kind | Role |
|---|------|------|------|
| 1 | **`InviteModal`** | Exported React component | Renders the invite dialog UI; wires props and local state to child handlers and effects. |
| 2 | **`handleKeyDown`** | Inner function (registered in `useEffect` on `document`) | Handles `Escape` (close) and `Tab` / `Shift+Tab` focus wrapping inside the dialog. |
| 3 | **`handleCopyLink`** | Async method on the component | Copies `shareLink` to the clipboard (API or fallback), then shows temporary “copied” feedback. |
| 4 | **`handleSend`** | Method on the component | Parses comma-separated emails, validates with `EMAIL_RE`, shows errors or calls `onSendInvites` and `onClose`. |
| 5 | **`handleKeyPress`** | Method on the component | On the email field, if key is `Enter`, delegates to the same behavior as **Send Invites** (`handleSend`). |

**Note:** The file also defines the constant **`EMAIL_RE`** (regex) and uses two **`useEffect`** hooks (focus on mount; attach/remove global key listener). Those are not separate named “functions” in the list above, but their behavior is covered by the tests below where relevant.

---

## Test table

For **component-level** rows, “inputs” are **props and user/environment actions**; “expected output” is **observable UI or callback behavior**.

| # | Purpose | Function / area | Test inputs | Expected output if the test passes |
|---|---------|-----------------|-------------|-----------------------------------|
| 1 | Renders trip context | `InviteModal` | `tripName="Summer Trip"`, valid `shareLink`, stub `onClose` / `onSendInvites` | Heading “Invite to Trip”, subtitle shows `tripName`, readonly share field shows `shareLink`. |
| 2 | Default role is Editor | `InviteModal` | Same as (1), initial render | “Editor” role control appears selected (`aria-pressed="true"` on Editor, false on Viewer). |
| 3 | Focus email field on open | `InviteModal` + mount effect | Mount modal | `#invite-emails` (or equivalent) receives focus after render. |
| 4 | Typing updates value and clears error | `InviteModal` (inline `onChange`) | After an error is shown, user types in email field | Input value matches typed text; error message is cleared (`error` empty / no alert region). |
| 5 | Close via header button | `InviteModal` | Click close (X) | `onClose` called once. |
| 6 | Close via backdrop | `InviteModal` | Click backdrop | `onClose` called once. |
| 7 | Close via Cancel | `InviteModal` | Click Cancel | `onClose` called once. |
| 8 | Escape closes modal | `handleKeyDown` | `keydown` with `key === 'Escape'` (bubbles to `document`) | `onClose` called. |
| 9 | Tab wraps from last to first focusable | `handleKeyDown` | Focus on last focusable in dialog, `Tab` (no shift), dialog has ≥2 focusables | `preventDefault` on event; focus moves to first focusable in dialog. |
| 10 | Shift+Tab wraps from first to last | `handleKeyDown` | Focus on first focusable, `Shift+Tab` | `preventDefault`; focus moves to last focusable. |
| 11 | No wrap when single focusable | `handleKeyDown` | Dialog with only one focusable (if achievable in test setup), `Tab` | No erroneous wrap (behavior: either no `preventDefault` or no crash; matches implementation when `focusable.length` is 0 or 1). |
| 12 | Copy uses Clipboard API when available | `handleCopyLink` | `navigator.clipboard.writeText` resolves; click Copy | `writeText` called with `shareLink`; UI shows copied state (e.g. “Copied!”, green styling, `aria-label` for copied). |
| 13 | Copy falls back when API fails | `handleCopyLink` | Mock `writeText` to reject; click Copy | Fallback path runs (`execCommand('copy')` or equivalent); copied feedback still appears. |
| 14 | Copied state resets | `handleCopyLink` | After successful copy | Within ~2500ms after copy, copied UI returns to default (not stuck on “Copied!”). |
| 15 | Send with no emails | `handleSend` | `emailInput` empty or only commas/spaces; trigger Send | Error: “Please enter at least one email address.”; `onSendInvites` not called; `onClose` not called; input refocused. |
| 16 | Send with invalid email(s) | `handleSend` | e.g. `"bad"`, or `"a@b.com, not-an-email"` | Error mentions invalid email(s), lists invalid parts; `onSendInvites` and `onClose` not called; input refocused. Singular “email” when one invalid, plural when multiple. |
| 17 | Send with valid single email | `handleSend` | `"user@example.com"`, role e.g. Viewer | `onSendInvites(["user@example.com"], "Viewer")`; `onClose` called. |
| 18 | Send with multiple valid emails | `handleSend` | `"a@x.com, b@y.com"` (with optional spaces) | `onSendInvites(["a@x.com","b@y.com"], role)`; `onClose` called. |
| 19 | Enter in email field sends | `handleKeyPress` → `handleSend` | Valid emails in field, `Enter` on `#invite-emails` | Same as successful send: `onSendInvites` with parsed emails and current role; `onClose`. |
| 20 | Enter with validation error | `handleKeyPress` → `handleSend` | Invalid or empty input, `Enter` | Same error behavior as Send button (no invite, no close). |
| 21 | Role selection before send | `InviteModal` | Select Viewer, then valid emails and Send | `onSendInvites` receives `role === 'Viewer'`. |
| 22 | Accessibility: error linked to input | `InviteModal` | After validation error | Input has `aria-invalid="true"` and `aria-describedby` pointing to error id; error in `role="alert"`. |

---

This specification can be mapped to **unit tests** (pure helpers if extracted), **component tests** (e.g. React Testing Library: render, user events, mocks for `navigator.clipboard` and callbacks), or a mix.
