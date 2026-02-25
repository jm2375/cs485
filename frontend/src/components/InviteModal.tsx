import { useState, useRef, useEffect } from 'react';
import type { KeyboardEvent } from 'react';
import { X, Copy, Check, Send } from 'lucide-react';
import type { Role } from '../types';

interface InviteModalProps {
  tripName: string;
  shareLink: string;
  onClose: () => void;
  onSendInvites: (emails: string[], role: Extract<Role, 'Editor' | 'Viewer'>) => void;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function InviteModal({ tripName, shareLink, onClose, onSendInvites }: InviteModalProps) {
  const [emailInput, setEmailInput] = useState('');
  const [role, setRole] = useState<'Editor' | 'Viewer'>('Editor');
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  // Focus trap: keep focus inside modal
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  useEffect(() => {
    function handleKeyDown(e: globalThis.KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  async function handleCopyLink() {
    try {
      await navigator.clipboard.writeText(shareLink);
    } catch {
      // Fallback for browsers without clipboard API
      const ta = document.createElement('textarea');
      ta.value = shareLink;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2500);
  }

  function handleSend() {
    const emails = emailInput
      .split(',')
      .map(e => e.trim())
      .filter(Boolean);

    if (emails.length === 0) {
      setError('Please enter at least one email address.');
      inputRef.current?.focus();
      return;
    }

    const invalid = emails.filter(e => !EMAIL_RE.test(e));
    if (invalid.length > 0) {
      setError(`Invalid email${invalid.length > 1 ? 's' : ''}: ${invalid.join(', ')}`);
      inputRef.current?.focus();
      return;
    }

    onSendInvites(emails, role);
    onClose();
  }

  function handleKeyPress(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') handleSend();
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 backdrop-blur-[2px] z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Modal */}
      <div
        className="fixed inset-0 z-50 flex items-center justify-center p-4"
        role="dialog"
        aria-modal="true"
        aria-labelledby="invite-title"
        ref={dialogRef}
      >
        <div className="bg-white rounded-2xl shadow-2xl w-full max-w-md ring-1 ring-black/5">
          {/* Header */}
          <div className="flex items-start justify-between px-6 pt-6 pb-4">
            <div>
              <h2 id="invite-title" className="text-xl font-semibold text-gray-900">
                Invite to Trip
              </h2>
              <p className="text-sm text-gray-400 mt-0.5">{tripName}</p>
            </div>
            <button
              onClick={onClose}
              className="p-1.5 rounded-full hover:bg-gray-100 transition-colors -mt-0.5 -mr-1"
              aria-label="Close invite dialog"
            >
              <X className="w-5 h-5 text-gray-400" />
            </button>
          </div>

          <div className="px-6 pb-6 space-y-5">
            {/* Email input */}
            <div>
              <label
                htmlFor="invite-emails"
                className="block text-sm font-medium text-gray-700 mb-1.5"
              >
                Email addresses
              </label>
              <input
                ref={inputRef}
                id="invite-emails"
                type="text"
                inputMode="email"
                autoComplete="email"
                value={emailInput}
                onChange={e => { setEmailInput(e.target.value); setError(''); }}
                onKeyDown={handleKeyPress}
                placeholder="Enter email address (comma separated)"
                className="w-full px-3.5 py-2.5 rounded-lg border border-gray-300 text-sm text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition-shadow"
                aria-describedby={error ? 'invite-error' : undefined}
                aria-invalid={!!error}
              />
              {error && (
                <p id="invite-error" role="alert" className="mt-1.5 text-xs text-red-600">
                  {error}
                </p>
              )}
            </div>

            {/* Role selector */}
            <div>
              <p className="text-sm font-medium text-gray-700 mb-2" id="role-label">
                Assign as:
              </p>
              <div
                className="flex gap-2"
                role="group"
                aria-labelledby="role-label"
              >
                {(['Editor', 'Viewer'] as const).map(r => (
                  <button
                    key={r}
                    type="button"
                    onClick={() => setRole(r)}
                    aria-pressed={role === r}
                    className={`flex-1 py-2.5 px-4 rounded-lg text-sm font-medium border transition-all ${
                      role === r
                        ? 'bg-gray-900 text-white border-gray-900'
                        : 'bg-white text-gray-600 border-gray-300 hover:border-gray-400 hover:bg-gray-50'
                    }`}
                  >
                    {r}
                  </button>
                ))}
              </div>
            </div>

            {/* Share link */}
            <div>
              <p className="text-sm font-medium text-gray-700 mb-2">Share Link</p>
              <div className="flex gap-2">
                <input
                  type="text"
                  readOnly
                  value={shareLink}
                  aria-label="Shareable trip link"
                  className="flex-1 min-w-0 px-3 py-2.5 rounded-lg border border-gray-200 bg-gray-50 text-xs text-gray-500 focus:outline-none cursor-default"
                />
                <button
                  onClick={handleCopyLink}
                  className={`flex items-center gap-1.5 px-3.5 py-2.5 rounded-lg text-sm font-medium border transition-all whitespace-nowrap ${
                    copied
                      ? 'bg-green-50 border-green-200 text-green-700'
                      : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
                  }`}
                  aria-label={copied ? 'Link copied to clipboard' : 'Copy shareable link'}
                >
                  {copied
                    ? <><Check className="w-4 h-4" aria-hidden="true" /> Copied!</>
                    : <><Copy className="w-4 h-4" aria-hidden="true" /> Copy Link</>
                  }
                </button>
              </div>
            </div>

            {/* Actions */}
            <div className="flex gap-3 pt-1">
              <button
                onClick={onClose}
                className="flex-1 py-2.5 px-4 rounded-lg border border-gray-300 text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleSend}
                className="flex-1 py-2.5 px-4 rounded-lg bg-blue-600 text-sm font-medium text-white hover:bg-blue-700 active:bg-blue-800 transition-colors flex items-center justify-center gap-2"
              >
                <Send className="w-4 h-4" aria-hidden="true" />
                Send Invites
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
