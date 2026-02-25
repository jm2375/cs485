import { useState, useRef, useEffect } from 'react';
import { MoreHorizontal } from 'lucide-react';
import type { Collaborator, Role } from '../types';
import { Avatar } from './Avatar';

const ROLE_ORDER: Role[] = ['Owner', 'Editor', 'Viewer'];

const ROLE_STYLES: Record<Role, string> = {
  Owner:  'bg-blue-100 text-blue-700',
  Editor: 'bg-green-100 text-green-700',
  Viewer: 'bg-gray-100 text-gray-600',
};

interface CollaboratorsPanelProps {
  collaborators: Collaborator[];
  onUpdateRole: (id: string, role: Role) => void;
  onRemove: (id: string) => void;
}

function ContextMenu({
  collaborator,
  onUpdateRole,
  onRemove,
  onClose,
}: {
  collaborator: Collaborator;
  onUpdateRole: (id: string, role: Role) => void;
  onRemove: (id: string) => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [onClose]);

  return (
    <div
      ref={ref}
      role="menu"
      aria-label={`Options for ${collaborator.name}`}
      className="absolute right-0 top-8 z-20 bg-white rounded-xl shadow-xl border border-gray-100 py-1.5 min-w-[160px]"
    >
      <p className="px-3 py-1 text-[10px] font-semibold text-gray-400 uppercase tracking-wider">
        Change role
      </p>
      {(['Editor', 'Viewer'] as Role[])
        .filter(r => r !== collaborator.role)
        .map(role => (
          <button
            key={role}
            role="menuitem"
            onClick={() => { onUpdateRole(collaborator.id, role); onClose(); }}
            className="w-full text-left px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 transition-colors"
          >
            Make {role}
          </button>
        ))}
      <div className="my-1 border-t border-gray-100" />
      <button
        role="menuitem"
        onClick={() => { onRemove(collaborator.id); onClose(); }}
        className="w-full text-left px-3 py-2 text-sm text-red-600 hover:bg-red-50 transition-colors"
      >
        Remove
      </button>
    </div>
  );
}

export function CollaboratorsPanel({
  collaborators,
  onUpdateRole,
  onRemove,
}: CollaboratorsPanelProps) {
  const [openMenuId, setOpenMenuId] = useState<string | null>(null);

  // Group and sort by role order
  const grouped = ROLE_ORDER.flatMap(role =>
    collaborators.filter(c => c.role === role)
  );

  return (
    <div className="flex flex-col h-full">
      {/* Section header */}
      <div className="flex items-center gap-3 px-4 py-3 border-b border-gray-100">
        <span className="text-sm font-semibold text-gray-700">
          {collaborators.length} collaborator{collaborators.length !== 1 ? 's' : ''}
        </span>
        <div className="flex -space-x-2" aria-label="Collaborator avatars">
          {collaborators.slice(0, 5).map(c => (
            <Avatar key={c.id} name={c.name} color={c.color} size="xs" avatarUrl={c.avatarUrl} />
          ))}
          {collaborators.length > 5 && (
            <div className="w-6 h-6 rounded-full bg-gray-200 ring-2 ring-white flex items-center justify-center text-[10px] text-gray-600 font-semibold">
              +{collaborators.length - 5}
            </div>
          )}
        </div>
      </div>

      {/* Collaborator rows grouped by role */}
      <ul className="flex-1 overflow-y-auto" aria-label="Collaborators list">
        {ROLE_ORDER.map(role => {
          const group = collaborators.filter(c => c.role === role);
          if (group.length === 0) return null;
          return (
            <li key={role}>
              {/* Role group header */}
              <div className="px-4 py-1.5 bg-gray-50 border-y border-gray-100">
                <span className={`text-[11px] font-semibold uppercase tracking-wider px-2 py-0.5 rounded-full ${ROLE_STYLES[role]}`}>
                  {role === 'Owner' ? 'Owner' : `${role}s`}
                </span>
              </div>
              <ul>
                {group.map(c => (
                  <li
                    key={c.id}
                    className="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 transition-colors border-b border-gray-50 last:border-b-0"
                  >
                    <Avatar name={c.name} color={c.color} size="sm" avatarUrl={c.avatarUrl} />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-gray-900 truncate">{c.name}</p>
                      <p className="text-xs text-gray-400 truncate">{c.email}</p>
                    </div>
                    {c.role !== 'Owner' && (
                      <div className="relative flex-shrink-0">
                        <button
                          onClick={() => setOpenMenuId(openMenuId === c.id ? null : c.id)}
                          className="p-1 rounded-md hover:bg-gray-100 transition-colors"
                          aria-label={`More options for ${c.name}`}
                          aria-expanded={openMenuId === c.id}
                          aria-haspopup="menu"
                        >
                          <MoreHorizontal className="w-4 h-4 text-gray-400" />
                        </button>
                        {openMenuId === c.id && (
                          <ContextMenu
                            collaborator={c}
                            onUpdateRole={onUpdateRole}
                            onRemove={onRemove}
                            onClose={() => setOpenMenuId(null)}
                          />
                        )}
                      </div>
                    )}
                  </li>
                ))}
              </ul>
            </li>
          );
        })}
        {grouped.length === 0 && (
          <li className="flex items-center justify-center h-24 text-sm text-gray-400">
            No collaborators yet
          </li>
        )}
      </ul>
    </div>
  );
}
