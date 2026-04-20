import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ItineraryItem, POI } from '../types';
import {
  ItineraryPanel,
  ItineraryCard,
  DeleteDayConfirm,
} from './ItineraryPanel';

function makePoi(id: string, name: string, overrides?: Partial<POI>): POI {
  return {
    id,
    name,
    category: 'restaurant',
    subcategory: 'Cafe',
    address: '1 Main St',
    rating: 4.5,
    reviewCount: 10,
    description: 'Description',
    imageUrl: 'https://example.com/photo.jpg',
    lat: 0,
    lng: 0,
    priceLevel: 2,
    ...overrides,
  };
}

function makeItem(
  id: string,
  day: number,
  poiName: string,
  opts?: { poi?: Partial<POI>; addedBy?: string }
): ItineraryItem {
  return {
    id,
    day,
    addedBy: opts?.addedBy ?? 'Alice',
    poi: makePoi(`poi-${id}`, poiName, opts?.poi),
  };
}

/** Gives each element a distinct vertical slot so @dnd-kit collision code can resolve targets in jsdom. */
function installDndLayoutMock() {
  const rects = new Map<Element, DOMRect>();
  let seq = 0;
  jest.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    if (!rects.has(this)) {
      const i = seq++;
      rects.set(this, new DOMRect(0, i * 90, 280, 80));
    }
    return rects.get(this)!;
  });
}

/**
 * One test per row in `ItineraryPanel.test-spec.md` (rows 1–20).
 * Nested `describe` blocks match functions in `ItineraryPanel.tsx`.
 */
describe('ItineraryPanel', () => {
  beforeEach(() => {
    jest.restoreAllMocks();
  });

  describe('ItineraryCard', () => {
    it('3. POI content on card', () => {
      const item = makeItem('x', 1, 'Museum', {
        poi: {
          subcategory: 'Museums',
          rating: 3.25,
          imageUrl: 'https://example.com/museum.jpg',
        },
        addedBy: 'Bob',
      });

      render(<ItineraryCard item={item} index={0} onRemove={jest.fn()} />);

      expect(screen.getByText('Museum')).toBeInTheDocument();
      expect(screen.getByText('Museums')).toBeInTheDocument();
      expect(screen.getByText('3.3')).toBeInTheDocument();
      expect(screen.getByText(/Added by Bob/)).toBeInTheDocument();
      expect(screen.getByRole('img', { name: 'Photo of Museum' })).toHaveAttribute(
        'src',
        'https://example.com/museum.jpg'
      );
    });

    it('4. Stop index in list', () => {
      const item = makeItem('x', 1, 'Place');
      const { rerender } = render(
        <ItineraryCard item={item} index={0} onRemove={jest.fn()} />
      );
      expect(screen.getByText('1')).toBeInTheDocument();

      rerender(<ItineraryCard item={item} index={2} onRemove={jest.fn()} />);
      expect(screen.getByText('3')).toBeInTheDocument();
    });

    it('5. Remove button', async () => {
      const user = userEvent.setup();
      const onRemove = jest.fn();
      const item = makeItem('item-42', 1, 'Removable Spot');

      render(<ItineraryCard item={item} index={0} onRemove={onRemove} />);

      await user.click(
        screen.getByRole('button', { name: 'Remove Removable Spot from itinerary' })
      );
      expect(onRemove).toHaveBeenCalledWith('item-42');
    });

    it('6. Remove hidden in overlay', () => {
      const item = makeItem('x', 1, 'Overlay POI');
      render(<ItineraryCard item={item} index={0} overlay />);

      expect(
        screen.queryByRole('button', { name: /Remove .* from itinerary/ })
      ).not.toBeInTheDocument();
    });

    it('7. Rating accessibility', () => {
      const item = makeItem('x', 1, 'Rated', { poi: { rating: 4 } });
      render(<ItineraryCard item={item} index={0} onRemove={jest.fn()} />);

      expect(screen.getByLabelText('Rating 4')).toBeInTheDocument();
    });
  });

  describe('SortableItem', () => {
    it('8. Drag handle present', () => {
      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'First Cafe')]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      expect(
        screen.getByRole('button', { name: 'Drag to reorder First Cafe' })
      ).toBeInTheDocument();
    });
  });

  describe('DeleteDayConfirm', () => {
    it('10. Delete day confirm copy', () => {
      const onConfirm = jest.fn();
      const onCancel = jest.fn();

      const { rerender } = render(
        <DeleteDayConfirm day={2} count={1} onConfirm={onConfirm} onCancel={onCancel} />
      );

      let dialog = screen.getByRole('alertdialog', { name: 'Confirm deletion of Day 2' });
      expect(within(dialog).getByText('Delete Day 2?')).toBeInTheDocument();
      expect(
        within(dialog).getByText('Removes 1 stop. Later days will be renumbered.')
      ).toBeInTheDocument();

      rerender(
        <DeleteDayConfirm day={2} count={3} onConfirm={onConfirm} onCancel={onCancel} />
      );
      dialog = screen.getByRole('alertdialog', { name: 'Confirm deletion of Day 2' });
      expect(
        within(dialog).getByText('Removes 3 stops. Later days will be renumbered.')
      ).toBeInTheDocument();
    });
  });

  describe('ItineraryPanel (component)', () => {
    it('1. Empty itinerary', () => {
      render(
        <ItineraryPanel
          itinerary={[]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      expect(screen.getByText('No places yet')).toBeInTheDocument();
      expect(
        screen.getByText('Switch to the Explore tab to find places to add.')
      ).toBeInTheDocument();
      expect(screen.queryByRole('region', { name: /Day / })).not.toBeInTheDocument();
    });

    it('2. Renders days and stops', () => {
      const itinerary: ItineraryItem[] = [
        makeItem('a', 1, 'First Cafe'),
        makeItem('b', 1, 'Second Park'),
        makeItem('c', 2, 'Museum'),
      ];

      render(
        <ItineraryPanel
          itinerary={itinerary}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      const day1 = screen.getByRole('region', { name: 'Day 1' });
      const day2 = screen.getByRole('region', { name: 'Day 2' });

      expect(within(day1).getByRole('heading', { name: 'Day 1' })).toBeInTheDocument();
      expect(within(day2).getByRole('heading', { name: 'Day 2' })).toBeInTheDocument();

      expect(within(day1).getByText('2 stops')).toBeInTheDocument();
      expect(within(day2).getByText('1 stop')).toBeInTheDocument();

      expect(
        within(day1).getByRole('list', { name: 'Stops for Day 1' })
      ).toBeInTheDocument();
      expect(
        within(day2).getByRole('list', { name: 'Stops for Day 2' })
      ).toBeInTheDocument();
    });

    it('9. Toggle delete-day confirmation', async () => {
      const user = userEvent.setup();
      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'Solo')]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      const deleteBtn = screen.getByRole('button', { name: 'Delete Day 1' });
      expect(deleteBtn).toHaveAttribute('aria-expanded', 'false');

      await user.click(deleteBtn);
      expect(deleteBtn).toHaveAttribute('aria-expanded', 'true');
      expect(
        screen.getByRole('alertdialog', { name: 'Confirm deletion of Day 1' })
      ).toBeInTheDocument();

      await user.click(deleteBtn);
      expect(deleteBtn).toHaveAttribute('aria-expanded', 'false');
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    });

    it('11. Confirm delete day', async () => {
      const user = userEvent.setup();
      const onDeleteDay = jest.fn();

      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'Solo')]}
          onRemove={jest.fn()}
          onDeleteDay={onDeleteDay}
          onReorder={jest.fn()}
        />
      );

      await user.click(screen.getByRole('button', { name: 'Delete Day 1' }));
      await user.click(screen.getByRole('button', { name: 'Delete Day' }));

      expect(onDeleteDay).toHaveBeenCalledWith(1);
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    });

    it('12. Cancel delete day', async () => {
      const user = userEvent.setup();
      const onDeleteDay = jest.fn();

      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'Solo')]}
          onRemove={jest.fn()}
          onDeleteDay={onDeleteDay}
          onReorder={jest.fn()}
        />
      );

      await user.click(screen.getByRole('button', { name: 'Delete Day 1' }));
      await user.click(screen.getByRole('button', { name: 'Cancel' }));

      expect(onDeleteDay).not.toHaveBeenCalled();
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    });

    it('20. Multiple days sorted', () => {
      const itinerary: ItineraryItem[] = [
        makeItem('b', 2, 'Later day first in array'),
        makeItem('a', 1, 'Earlier day second in array'),
      ];

      render(
        <ItineraryPanel
          itinerary={itinerary}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      const regions = screen.getAllByRole('region', { name: /Day \d+/ });
      expect(regions.map(r => r.getAttribute('aria-label'))).toEqual(['Day 1', 'Day 2']);
    });
  });

  describe('useEffect (itinerary sync)', () => {
    it('13. Prop sync when not dragging', () => {
      const initial = [makeItem('a', 1, 'Old Name')];
      const { rerender } = render(
        <ItineraryPanel
          itinerary={initial}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      expect(screen.getByText('Old Name')).toBeInTheDocument();

      rerender(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'New Name')]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      expect(screen.getByText('New Name')).toBeInTheDocument();
      expect(screen.queryByText('Old Name')).not.toBeInTheDocument();
    });

    it('14. Prop sync suppressed during drag', async () => {
      const user = userEvent.setup();
      installDndLayoutMock();

      const v1 = [makeItem('a', 1, 'First'), makeItem('b', 1, 'Second')];
      const v2 = [makeItem('z', 1, 'Replaced')];

      const { rerender } = render(
        <ItineraryPanel
          itinerary={v1}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      const handleA = screen.getByRole('button', { name: /Drag to reorder First/ });
      await user.pointer([
        { keys: '[MouseLeft>]', target: handleA, coords: { x: 10, y: 10 } },
        { coords: { x: 10, y: 40 } },
      ]);

      await waitFor(() => {
        expect(screen.getAllByText('First').length).toBeGreaterThan(0);
      });

      rerender(
        <ItineraryPanel
          itinerary={v2}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      expect(screen.queryByText('Replaced')).not.toBeInTheDocument();
      expect(screen.getByText('2 stops')).toBeInTheDocument();
    });
  });

  describe('handleDragStart', () => {
    it('15. handleDragStart sets active', async () => {
      const user = userEvent.setup();
      installDndLayoutMock();

      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'DragMe'), makeItem('b', 1, 'Other')]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      const handle = screen.getByRole('button', { name: /Drag to reorder DragMe/ });
      await user.pointer([
        { keys: '[MouseLeft>]', target: handle, coords: { x: 5, y: 5 } },
        { coords: { x: 20, y: 5 } },
      ]);

      await waitFor(() => {
        expect(screen.getAllByText('DragMe').length).toBeGreaterThanOrEqual(1);
      });
      expect(document.querySelector('.ring-blue-500')).toBeTruthy();
    });
  });

  describe('handleDragOver', () => {
    it('18. handleDragOver within-day reorder', async () => {
      const user = userEvent.setup();
      installDndLayoutMock();

      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'First'), makeItem('b', 1, 'Second')]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={jest.fn()}
        />
      );

      const list = screen.getByRole('list', { name: 'Stops for Day 1' });
      const h1 = screen.getByRole('button', { name: /Drag to reorder First/ });
      const h2 = screen.getByRole('button', { name: /Drag to reorder Second/ });

      expect(
        within(list).getAllByRole('img', { name: /Photo of/ }).map(i => i.getAttribute('alt'))
      ).toEqual(['Photo of First', 'Photo of Second']);

      await user.pointer([
        { keys: '[MouseLeft>]', target: h1, coords: { x: 10, y: 10 } },
        { target: h2, coords: { x: 10, y: 100 } },
      ]);

      await waitFor(() => {
        const alts = within(list)
          .getAllByRole('img', { name: /Photo of/ })
          .map(i => i.getAttribute('alt'));
        expect(alts).toEqual(['Photo of Second', 'Photo of First']);
      });
    });

    it('19. handleDragOver cross-day move', async () => {
      const user = userEvent.setup();
      installDndLayoutMock();
      const onReorder = jest.fn();

      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'MoveMe'), makeItem('b', 2, 'Target')]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={onReorder}
        />
      );

      const hA = screen.getByRole('button', { name: /Drag to reorder MoveMe/ });
      const hB = screen.getByRole('button', { name: /Drag to reorder Target/ });

      await user.pointer([
        { keys: '[MouseLeft>]', target: hA, coords: { x: 10, y: 10 } },
        { target: hB, coords: { x: 10, y: 200 } },
        { keys: '[/MouseLeft]', target: hB, coords: { x: 10, y: 200 } },
      ]);

      await waitFor(() => {
        expect(onReorder).toHaveBeenCalled();
      });

      const arg = onReorder.mock.calls[onReorder.mock.calls.length - 1][0] as ItineraryItem[];
      expect(arg.find(i => i.id === 'a')?.day).toBe(2);
    });
  });

  describe('handleDragEnd', () => {
    it('16. handleDragEnd commits reorder', async () => {
      const user = userEvent.setup();
      installDndLayoutMock();
      const onReorder = jest.fn();

      render(
        <ItineraryPanel
          itinerary={[makeItem('a', 1, 'First'), makeItem('b', 1, 'Second')]}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={onReorder}
        />
      );

      const h1 = screen.getByRole('button', { name: /Drag to reorder First/ });
      const h2 = screen.getByRole('button', { name: /Drag to reorder Second/ });

      await user.pointer([
        { keys: '[MouseLeft>]', target: h1, coords: { x: 10, y: 10 } },
        { target: h2, coords: { x: 10, y: 100 } },
        { keys: '[/MouseLeft]', target: h2, coords: { x: 10, y: 100 } },
      ]);

      await waitFor(() => {
        expect(onReorder).toHaveBeenCalled();
      });

      const arg = onReorder.mock.calls[onReorder.mock.calls.length - 1][0] as ItineraryItem[];
      expect(arg.map(i => i.id)).toEqual(['b', 'a']);
    });

    it('17. handleDragEnd cancels without drop target', async () => {
      const user = userEvent.setup();
      installDndLayoutMock();
      const onReorder = jest.fn();
      const itinerary = [makeItem('a', 1, 'Only')];

      render(
        <ItineraryPanel
          itinerary={itinerary}
          onRemove={jest.fn()}
          onDeleteDay={jest.fn()}
          onReorder={onReorder}
        />
      );

      const handle = screen.getByRole('button', { name: /Drag to reorder Only/ });
      await user.pointer([
        { keys: '[MouseLeft>]', target: handle, coords: { x: 5, y: 5 } },
        { coords: { x: 5, y: 5 } },
        { keys: '[/MouseLeft]', target: document.body, coords: { x: 5, y: 5 } },
      ]);

      await waitFor(() => {
        expect(onReorder).not.toHaveBeenCalled();
      });
      expect(screen.getByText('Only')).toBeInTheDocument();
    });
  });
});
