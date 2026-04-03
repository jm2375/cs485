import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { InviteModal } from './InviteModal';

const tripName = 'Summer Trip';
const shareLink = 'https://example.com/trip/abc';

function renderInviteModal(overrides?: {
  tripName?: string;
  shareLink?: string;
  onClose?: () => void;
  onSendInvites?: (emails: string[], role: 'Editor' | 'Viewer') => void;
}) {
  const onClose = overrides?.onClose ?? jest.fn();
  const onSendInvites = overrides?.onSendInvites ?? jest.fn();
  return {
    onClose,
    onSendInvites,
    ...render(
      <InviteModal
        tripName={overrides?.tripName ?? tripName}
        shareLink={overrides?.shareLink ?? shareLink}
        onClose={onClose}
        onSendInvites={onSendInvites}
      />
    ),
  };
}

describe('InviteModal', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useRealTimers();
  });

  describe('InviteModal (component)', () => {
    it('renders trip context', () => {
      const { onClose, onSendInvites } = renderInviteModal();

      expect(screen.getByRole('heading', { name: 'Invite to Trip' })).toBeInTheDocument();
      expect(screen.getByText('Summer Trip')).toBeInTheDocument();
      expect(screen.getByLabelText('Shareable trip link')).toHaveValue(shareLink);
      expect(onClose).not.toHaveBeenCalled();
      expect(onSendInvites).not.toHaveBeenCalled();
    });

    it('default role is Editor', () => {
      renderInviteModal();

      const editor = screen.getByRole('button', { name: 'Editor' });
      const viewer = screen.getByRole('button', { name: 'Viewer' });

      expect(editor).toHaveAttribute('aria-pressed', 'true');
      expect(viewer).toHaveAttribute('aria-pressed', 'false');
    });

    it('focuses email field on open', async () => {
      renderInviteModal();
      const email = screen.getByLabelText('Email addresses');
      await waitFor(() => expect(email).toHaveFocus());
    });

    it('typing updates value and clears error after validation error', async () => {
      renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Please enter at least one email address.'
      );

      fireEvent.change(email, { target: { value: 'a@b.com' } });
      expect(email).toHaveValue('a@b.com');
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });

    it('closes via header button', () => {
      const { onClose } = renderInviteModal();
      fireEvent.click(screen.getByRole('button', { name: 'Close invite dialog' }));
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('closes via backdrop', () => {
      const { container, onClose } = renderInviteModal();
      const backdrop = container.querySelector('[aria-hidden="true"]');
      expect(backdrop).toBeTruthy();
      fireEvent.click(backdrop!);
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('closes via Cancel', () => {
      const { onClose } = renderInviteModal();
      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('links validation error to input for accessibility', async () => {
      renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));
      const alert = await screen.findByRole('alert');

      expect(email).toHaveAttribute('aria-invalid', 'true');
      expect(email.getAttribute('aria-describedby')).toBe('invite-error');
      expect(alert).toHaveAttribute('id', 'invite-error');
    });
  });

  describe('handleKeyDown', () => {
    it('closes on Escape', () => {
      const { onClose } = renderInviteModal();
      fireEvent.keyDown(document, { key: 'Escape', bubbles: true });
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('wraps Tab from last focusable to first', () => {
      renderInviteModal();
      const send = screen.getByRole('button', { name: 'Send Invites' });
      const close = screen.getByRole('button', { name: 'Close invite dialog' });

      send.focus();
      const evt = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
      const preventDefault = jest.spyOn(evt, 'preventDefault');
      document.dispatchEvent(evt);

      expect(preventDefault).toHaveBeenCalled();
      expect(close).toHaveFocus();
    });

    it('wraps Shift+Tab from first focusable to last', () => {
      renderInviteModal();
      const send = screen.getByRole('button', { name: 'Send Invites' });
      const close = screen.getByRole('button', { name: 'Close invite dialog' });

      close.focus();
      const evt = new KeyboardEvent('keydown', {
        key: 'Tab',
        shiftKey: true,
        bubbles: true,
        cancelable: true,
      });
      const preventDefault = jest.spyOn(evt, 'preventDefault');
      document.dispatchEvent(evt);

      expect(preventDefault).toHaveBeenCalled();
      expect(send).toHaveFocus();
    });

    it('does not preventDefault on Tab when focus is not on first or last focusable', () => {
      renderInviteModal();
      const email = screen.getByLabelText('Email addresses');
      email.focus();

      const evt = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
      const preventDefault = jest.spyOn(evt, 'preventDefault');
      document.dispatchEvent(evt);

      expect(preventDefault).not.toHaveBeenCalled();
    });
  });

  describe('handleCopyLink', () => {
    it('uses Clipboard API when available', async () => {
      const writeText = jest.fn().mockResolvedValue(undefined);
      Object.assign(navigator, { clipboard: { writeText } });

      renderInviteModal();
      fireEvent.click(screen.getByRole('button', { name: 'Copy shareable link' }));

      await waitFor(() => {
        expect(writeText).toHaveBeenCalledWith(shareLink);
      });
      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: 'Link copied to clipboard' })
        ).toBeInTheDocument();
      });
    });

    it('falls back when Clipboard API fails', async () => {
      const writeText = jest.fn().mockRejectedValue(new Error('denied'));
      Object.assign(navigator, { clipboard: { writeText } });
      const execCommand = jest.fn().mockReturnValue(true);
      Object.defineProperty(document, 'execCommand', {
        value: execCommand,
        writable: true,
        configurable: true,
      });

      renderInviteModal();
      fireEvent.click(screen.getByRole('button', { name: 'Copy shareable link' }));

      await waitFor(() => {
        expect(execCommand).toHaveBeenCalledWith('copy');
      });
      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: 'Link copied to clipboard' })
        ).toBeInTheDocument();
      });
    });

    it('resets copied state after delay', async () => {
      jest.useFakeTimers();
      const writeText = jest.fn().mockResolvedValue(undefined);
      Object.assign(navigator, { clipboard: { writeText } });

      renderInviteModal();
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Copy shareable link' }));
      });
      await act(async () => {
        await Promise.resolve();
      });

      expect(screen.getByRole('button', { name: 'Link copied to clipboard' })).toBeInTheDocument();

      await act(async () => {
        jest.advanceTimersByTime(2500);
      });

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Copy shareable link' })).toBeInTheDocument();
      });
    });
  });

  describe('handleSend', () => {
    it('shows error when there are no emails and does not call callbacks', async () => {
      const { onClose, onSendInvites } = renderInviteModal();

      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Please enter at least one email address.'
      );
      expect(onSendInvites).not.toHaveBeenCalled();
      expect(onClose).not.toHaveBeenCalled();

      const email = screen.getByLabelText('Email addresses');
      fireEvent.change(email, { target: { value: '   ,  , ' } });
      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Please enter at least one email address.'
      );
    });

    it('shows error for invalid emails with singular or plural wording', async () => {
      const { onClose, onSendInvites } = renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.change(email, { target: { value: 'bad' } });
      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));
      expect(await screen.findByRole('alert')).toHaveTextContent('Invalid email: bad');
      expect(onSendInvites).not.toHaveBeenCalled();
      expect(onClose).not.toHaveBeenCalled();

      fireEvent.change(email, { target: { value: 'a@b.com, not-an-email, also-bad' } });
      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Invalid emails: not-an-email, also-bad'
      );
    });

    it('calls onSendInvites with Editor and closes for a valid single email', () => {
      const { onClose, onSendInvites } = renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.change(email, { target: { value: 'user@example.com' } });
      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));

      expect(onSendInvites).toHaveBeenCalledWith(['user@example.com'], 'Editor');
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('parses multiple valid emails and closes', () => {
      const { onClose, onSendInvites } = renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.change(email, { target: { value: 'a@x.com,  b@y.com' } });
      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));

      expect(onSendInvites).toHaveBeenCalledWith(['a@x.com', 'b@y.com'], 'Editor');
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('passes Viewer role when selected', () => {
      const { onClose, onSendInvites } = renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.click(screen.getByRole('button', { name: 'Viewer' }));
      fireEvent.change(email, { target: { value: 'user@example.com' } });
      fireEvent.click(screen.getByRole('button', { name: 'Send Invites' }));

      expect(onSendInvites).toHaveBeenCalledWith(['user@example.com'], 'Viewer');
      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  describe('handleKeyPress', () => {
    it('submits on Enter like the Send button', () => {
      const { onClose, onSendInvites } = renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.change(email, { target: { value: 'ok@example.com' } });
      fireEvent.keyDown(email, { key: 'Enter' });

      expect(onSendInvites).toHaveBeenCalledWith(['ok@example.com'], 'Editor');
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('applies validation on Enter like the Send button', () => {
      const { onClose, onSendInvites } = renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.keyDown(email, { key: 'Enter' });
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Please enter at least one email address.'
      );
      expect(onSendInvites).not.toHaveBeenCalled();
      expect(onClose).not.toHaveBeenCalled();

      fireEvent.change(email, { target: { value: 'nope' } });
      fireEvent.keyDown(email, { key: 'Enter' });
      expect(screen.getByRole('alert')).toHaveTextContent('Invalid email: nope');
      expect(onSendInvites).not.toHaveBeenCalled();
      expect(onClose).not.toHaveBeenCalled();
    });

    it('does not submit when the key is not Enter', () => {
      const { onClose, onSendInvites } = renderInviteModal();
      const email = screen.getByLabelText('Email addresses');

      fireEvent.change(email, { target: { value: 'ok@example.com' } });
      fireEvent.keyDown(email, { key: 'ArrowDown' });

      expect(onSendInvites).not.toHaveBeenCalled();
      expect(onClose).not.toHaveBeenCalled();
    });
  });
});
