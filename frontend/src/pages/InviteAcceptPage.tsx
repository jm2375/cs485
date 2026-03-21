import { useState, useEffect } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { MapPin, Users, Loader2 } from 'lucide-react';
import { api } from '../api';

type PageState = 'loading' | 'preview' | 'joining' | 'error';

interface TripPreview {
  tripId: string;
  name: string;
  destination: string;
  role?: string;
}

// Handles both /invite/:inviteCode (shareable link) and /accept/:token (email invite)
export default function InviteAcceptPage({ mode }: { mode: 'link' | 'token' }) {
  const { inviteCode, token } = useParams<{ inviteCode?: string; token?: string }>();
  const navigate = useNavigate();

  const [state, setState]     = useState<PageState>('loading');
  const [preview, setPreview] = useState<TripPreview | null>(null);
  const [errorMsg, setErrorMsg] = useState('');

  const param = mode === 'link' ? inviteCode! : token!;
  const isAuthed = api.isAuthenticated();

  useEffect(() => {
    async function load() {
      try {
        if (mode === 'link') {
          const data = await api.getShareLinkPreview(param);
          setPreview({ tripId: data.tripId, name: data.name, destination: data.destination });
        } else {
          const data = await api.getInvitePreview(param);
          setPreview({ tripId: data.tripId, name: data.tripName, destination: data.destination, role: data.role });
        }
        setState('preview');
      } catch {
        setErrorMsg('This invite link is invalid or has expired.');
        setState('error');
      }
    }
    load();
  }, [mode, param]);

  async function handleJoin() {
    if (!isAuthed) {
      const returnUrl = encodeURIComponent(window.location.pathname);
      navigate(`/login?return=${returnUrl}`);
      return;
    }
    setState('joining');
    try {
      if (mode === 'link') {
        await api.joinByInviteCode(param);
      } else {
        await api.acceptInvitation(param);
      }
      // Store the trip_id so App.tsx can load it directly for this user.
      localStorage.setItem('trip_id', preview.tripId);
      localStorage.removeItem('bootstrap_v1');
      navigate('/', { replace: true });
    } catch (err) {
      setErrorMsg((err as Error).message ?? 'Failed to join trip.');
      setState('error');
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center p-4">
      <div className="bg-white rounded-2xl shadow-sm border border-gray-200 w-full max-w-sm p-8">

        {state === 'loading' && (
          <div className="flex flex-col items-center gap-3 py-6">
            <Loader2 className="w-8 h-8 text-blue-500 animate-spin" />
            <p className="text-sm text-gray-500">Loading invite…</p>
          </div>
        )}

        {state === 'error' && (
          <div className="flex flex-col items-center gap-4 py-4 text-center">
            <div className="w-12 h-12 rounded-full bg-red-100 flex items-center justify-center">
              <span className="text-2xl">🔗</span>
            </div>
            <div>
              <h1 className="text-lg font-semibold text-gray-900">Invite unavailable</h1>
              <p className="text-sm text-gray-500 mt-1">{errorMsg}</p>
            </div>
            <Link to="/" className="text-sm text-blue-600 hover:underline">Back to dashboard</Link>
          </div>
        )}

        {(state === 'preview' || state === 'joining') && preview && (
          <>
            <div className="flex flex-col items-center gap-3 mb-6 text-center">
              <div className="w-14 h-14 rounded-full bg-blue-100 flex items-center justify-center text-3xl">✈️</div>
              <div>
                <h1 className="text-xl font-semibold text-gray-900">{preview.name}</h1>
                {preview.destination && (
                  <p className="flex items-center justify-center gap-1 text-sm text-gray-500 mt-0.5">
                    <MapPin className="w-3.5 h-3.5" />
                    {preview.destination}
                  </p>
                )}
              </div>
            </div>

            {preview.role && (
              <p className="text-sm text-center text-gray-600 mb-4">
                You've been invited as <span className="font-medium text-gray-900">{preview.role}</span>
              </p>
            )}

            {!isAuthed && (
              <p className="text-sm text-center text-amber-700 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2 mb-4">
                You need to sign in before joining.
              </p>
            )}

            <button
              onClick={handleJoin}
              disabled={state === 'joining'}
              className="w-full flex items-center justify-center gap-2 py-2.5 rounded-lg bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
            >
              {state === 'joining'
                ? <><Loader2 className="w-4 h-4 animate-spin" /> Joining…</>
                : <><Users className="w-4 h-4" /> {isAuthed ? 'Join Trip' : 'Sign in to Join'}</>
              }
            </button>

            <p className="text-xs text-center text-gray-400 mt-4">
              <Link to="/" className="hover:underline">Back to dashboard</Link>
            </p>
          </>
        )}
      </div>
    </div>
  );
}
