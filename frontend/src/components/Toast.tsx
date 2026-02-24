import { useEffect } from 'react';
import { CheckCircle, Info, X } from 'lucide-react';
import type { Toast as ToastType } from '../types';

interface ToastProps {
  toast: ToastType;
  onDismiss: (id: string) => void;
}

const ICONS = {
  success: <CheckCircle className="w-4 h-4 text-green-500" aria-hidden="true" />,
  info:    <Info        className="w-4 h-4 text-blue-500"  aria-hidden="true" />,
  error:   <Info        className="w-4 h-4 text-red-500"   aria-hidden="true" />,
};

const BG = {
  success: 'bg-white border-green-200',
  info:    'bg-white border-blue-200',
  error:   'bg-white border-red-200',
};

export function ToastNotification({ toast, onDismiss }: ToastProps) {
  useEffect(() => {
    const timer = setTimeout(() => onDismiss(toast.id), 3500);
    return () => clearTimeout(timer);
  }, [toast.id, onDismiss]);

  return (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="true"
      className={`flex items-center gap-3 px-4 py-3 rounded-xl shadow-lg border text-sm text-gray-800 max-w-sm animate-in ${BG[toast.type]}`}
    >
      {ICONS[toast.type]}
      <span className="flex-1">{toast.message}</span>
      <button
        onClick={() => onDismiss(toast.id)}
        className="p-0.5 rounded-md hover:bg-gray-100 transition-colors flex-shrink-0"
        aria-label="Dismiss notification"
      >
        <X className="w-3.5 h-3.5 text-gray-400" />
      </button>
    </div>
  );
}

interface ToastContainerProps {
  toasts: ToastType[];
  onDismiss: (id: string) => void;
}

export function ToastContainer({ toasts, onDismiss }: ToastContainerProps) {
  if (toasts.length === 0) return null;
  return (
    <div
      className="fixed bottom-6 left-1/2 -translate-x-1/2 z-[9999] flex flex-col gap-2 items-center"
      aria-label="Notifications"
    >
      {toasts.map(t => (
        <ToastNotification key={t.id} toast={t} onDismiss={onDismiss} />
      ))}
    </div>
  );
}
