import { useState, useRef, useEffect } from 'react';
import { MoreHorizontal, Plus, Settings } from 'lucide-react';
import type { Collaborator, Role } from '../types';
import { Avatar } from './Avatar';

const ROLE_STYLES: Record<Role, string> = {
  Owner:  'bg-blue-100 text-blue-700',
  Editor: 'bg-green-100 text-green-700',
  Viewer: 'bg-gray-100 text-gray-600',
};

interface CollaboratorsPanelProps {
  collaborators: Collaborator[];
  onOpenInvite: () => void;
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
  onOpenInvite,
  onUpdateRole,
  onRemove,
}: CollaboratorsPanelProps) {
  const [openMenuId, setOpenMenuId] = useState<string | null>(null);

  return (
    <div className="flex flex-col h-full">
      {/* Section header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100">
        <div className="flex items-center gap-3">
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
        <button
          onClick={onOpenInvite}
          className="flex items-center gap-1.5 text-xs font-medium text-blue-600 hover:text-blue-700 px-2.5 py-1.5 rounded-lg hover:bg-blue-50 transition-colors"
          aria-label="Invite a collaborator"
        >
          <Plus className="w-3.5 h-3.5" aria-hidden="true" />
          Invite
        </button>
      </div>

      {/* Collaborator rows */}
      <ul className="flex-1 overflow-y-auto divide-y divide-gray-50" aria-label="Collaborators list">
        {collaborators.map(c => (
          <li
            key={c.id}
            className="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 transition-colors"
          >
            <Avatar name={c.name} color={c.color} size="sm" avatarUrl={c.avatarUrl} />
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-gray-900 truncate">{c.name}</p>
              <p className="text-xs text-gray-400 truncate">{c.email}</p>
            </div>
            <div className="flex items-center gap-2 flex-shrink-0">
              <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${ROLE_STYLES[c.role]}`}>
                {c.role}
              </span>
              {c.role !== 'Owner' && (
                <div className="relative">
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
            </div>
          </li>
        ))}
      </ul>

      {/* Footer */}
      <div className="px-4 py-3 border-t border-gray-100">
        <button
          className="flex items-center gap-2 text-xs font-medium text-gray-500 hover:text-gray-700 transition-colors"
          aria-label="Manage permissions"
        >
          <Settings className="w-3.5 h-3.5" aria-hidden="true" />
          Manage Permissions
        </button>
      </div>
    </div>
  );
}
