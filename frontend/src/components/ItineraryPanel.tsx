import { useState, useEffect } from 'react';
import {
  DndContext,
  DragOverlay,
  closestCenter,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  type DragStartEvent,
  type DragOverEvent,
  type DragEndEvent,
  type UniqueIdentifier,
} from '@dnd-kit/core';
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Star, MapPin, Trash2, GripVertical, AlertTriangle } from 'lucide-react';
import type { ItineraryItem } from '../types';

// ─── Shared card renderer ────────────────────────────────────────────────────

interface CardProps {
  item: ItineraryItem;
  index: number;
  onRemove?: (id: string) => void;
  /** When true: render as the floating DragOverlay card */
  overlay?: boolean;
  /** When true: the original slot is dimmed while being dragged */
  isDragging?: boolean;
}

export function ItineraryCard({ item, index, onRemove, overlay, isDragging }: CardProps) {
  return (
    <div
      className={`flex items-start gap-3 px-3 py-3 transition-all ${
        overlay
          ? 'bg-white rounded-xl shadow-2xl ring-2 ring-blue-500 w-80'
          : isDragging
          ? 'opacity-30 bg-blue-50 rounded-lg'
          : 'hover:bg-gray-50 border-b border-gray-50 last:border-b-0'
      }`}
    >
      <div className="flex-shrink-0 mt-0.5">
        {overlay ? (
          <GripVertical className="w-4 h-4 text-blue-500" aria-hidden="true" />
        ) : (
          <div className="w-5 h-5 rounded-full border-2 border-gray-300 flex items-center justify-center text-[10px] font-bold text-gray-500">
            {index + 1}
          </div>
        )}
      </div>

      <img
        src={item.poi.imageUrl}
        alt={`Photo of ${item.poi.name}`}
        className="w-16 h-14 rounded-lg object-cover flex-shrink-0 bg-gray-100"
        loading="lazy"
        width={64}
        height={56}
      />

      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-gray-900 truncate">{item.poi.name}</p>
        <p className="text-[11px] text-gray-500 truncate">{item.poi.subcategory}</p>
        <span
          className="flex items-center gap-0.5 mt-0.5"
          aria-label={`Rating ${item.poi.rating}`}
        >
          <Star className="w-3 h-3 fill-amber-400 text-amber-400" aria-hidden="true" />
          <span className="text-xs text-gray-600">{item.poi.rating.toFixed(1)}</span>
        </span>
        <p className="text-[11px] text-gray-400 mt-0.5">Added by {item.addedBy}</p>
      </div>

      {!overlay && onRemove && (
        <button
          onClick={() => onRemove(item.id)}
          className="p-1.5 rounded-lg opacity-0 group-hover/item:opacity-100 focus:opacity-100 focus-visible:opacity-100 hover:bg-red-50 text-gray-400 hover:text-red-500 transition-all flex-shrink-0"
          aria-label={`Remove ${item.poi.name} from itinerary`}
        >
          <Trash2 className="w-4 h-4" />
        </button>
      )}
    </div>
  );
}

// ─── Sortable item wrapper ────────────────────────────────────────────────────

function SortableItem({
  item,
  index,
  onRemove,
}: {
  item: ItineraryItem;
  index: number;
  onRemove: (id: string) => void;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <li
      ref={setNodeRef}
      style={style}
      className="flex group/item"
      {...attributes}
    >
      {/* Grip handle — only this element activates the drag */}
      <button
        ref={setActivatorNodeRef}
        {...listeners}
        className="flex items-center px-2 text-gray-300 hover:text-gray-500 cursor-grab active:cursor-grabbing touch-none"
        aria-label={`Drag to reorder ${item.poi.name}`}
        tabIndex={0}
      >
        <GripVertical className="w-4 h-4" aria-hidden="true" />
      </button>

      <div className="flex-1 min-w-0">
        <ItineraryCard item={item} index={index} onRemove={onRemove} isDragging={isDragging} />
      </div>
    </li>
  );
}

// ─── Day delete confirmation ──────────────────────────────────────────────────

export function DeleteDayConfirm({
  day,
  count,
  onConfirm,
  onCancel,
}: {
  day: number;
  count: number;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <div
      className="mx-3 mb-2 mt-1 flex items-start gap-2 bg-red-50 border border-red-200 rounded-lg px-3 py-2.5"
      role="alertdialog"
      aria-label={`Confirm deletion of Day ${day}`}
    >
      <AlertTriangle className="w-4 h-4 text-red-500 flex-shrink-0 mt-0.5" aria-hidden="true" />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-semibold text-red-700">Delete Day {day}?</p>
        <p className="text-[11px] text-red-500 mt-0.5">
          Removes {count} stop{count !== 1 ? 's' : ''}. Later days will be renumbered.
        </p>
        <div className="flex gap-2 mt-2">
          <button
            onClick={onConfirm}
            className="text-[11px] font-semibold px-2.5 py-1 rounded-md bg-red-600 text-white hover:bg-red-700 transition-colors"
          >
            Delete Day
          </button>
          <button
            onClick={onCancel}
            className="text-[11px] font-medium px-2.5 py-1 rounded-md bg-white border border-gray-200 text-gray-600 hover:bg-gray-50 transition-colors"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── Main panel ───────────────────────────────────────────────────────────────

interface ItineraryPanelProps {
  itinerary: ItineraryItem[];
  onRemove: (id: string) => void;
  onDeleteDay: (day: number) => void;
  onReorder: (items: ItineraryItem[]) => void;
}

export function ItineraryPanel({
  itinerary,
  onRemove,
  onDeleteDay,
  onReorder,
}: ItineraryPanelProps) {
  // Local working copy — updated live during drag for smooth previews
  const [items, setItems] = useState<ItineraryItem[]>(itinerary);
  const [activeId, setActiveId]     = useState<UniqueIdentifier | null>(null);
  const [confirmDay, setConfirmDay] = useState<number | null>(null);

  // Sync from props only when not in the middle of a drag
  useEffect(() => {
    if (!activeId) setItems(itinerary);
  }, [itinerary, activeId]);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  // Group items by day for rendering
  const byDay = items.reduce<Record<number, ItineraryItem[]>>((acc, item) => {
    (acc[item.day] ??= []).push(item);
    return acc;
  }, {});
  const days = Object.keys(byDay).map(Number).sort((a, b) => a - b);

  const activeItem = activeId ? items.find(i => i.id === String(activeId)) : null;

  // ── Drag handlers ──────────────────────────────────────────────────────────

  function handleDragStart({ active }: DragStartEvent) {
    setActiveId(active.id);
  }

  /**
   * onDragOver fires continuously during drag.
   * Handles both within-day reordering (live gap preview) and cross-day moves.
   */
  function handleDragOver({ active, over }: DragOverEvent) {
    if (!over || active.id === over.id) return;

    const activeId = String(active.id);
    const overId   = String(over.id);

    setItems(prev => {
      const activeItem = prev.find(i => i.id === activeId);
      const overItem   = prev.find(i => i.id === overId);
      if (!activeItem || !overItem) return prev;

      // If crossing days, update the dragged item's day
      const withUpdatedDay = activeItem.day !== overItem.day
        ? prev.map(i => i.id === activeId ? { ...i, day: overItem.day } : i)
        : prev;

      const oldIdx = withUpdatedDay.findIndex(i => i.id === activeId);
      const newIdx = withUpdatedDay.findIndex(i => i.id === overId);

      if (oldIdx === newIdx || oldIdx === -1 || newIdx === -1) return withUpdatedDay;
      return arrayMove(withUpdatedDay, oldIdx, newIdx);
    });
  }

  function handleDragEnd({ over }: DragEndEvent) {
    setActiveId(null);
    if (!over) {
      // Cancelled — revert to last committed state
      setItems(itinerary);
      return;
    }
    // Commit whatever the live preview settled on (functional update sees latest items from handleDragOver)
    setItems(prev => {
      onReorder(prev);
      return prev;
    });
  }

  // ── Empty state ────────────────────────────────────────────────────────────

  if (itinerary.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 px-8 py-12 text-center">
        <div className="w-14 h-14 rounded-full bg-gray-100 flex items-center justify-center">
          <MapPin className="w-7 h-7 text-gray-400" aria-hidden="true" />
        </div>
        <div>
          <p className="text-sm font-semibold text-gray-600">No places yet</p>
          <p className="text-xs text-gray-400 mt-1">
            Switch to the Explore tab to find places to add.
          </p>
        </div>
      </div>
    );
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragEnd={handleDragEnd}
    >
      <div className="flex-1 overflow-y-auto">
        {days.map(day => (
          <section key={day} aria-label={`Day ${day}`}>
            {/* Day header */}
            <div className="sticky top-0 z-10 bg-gray-50 border-b border-gray-200 px-3 py-2 flex items-center gap-2 group">
              <div className="w-6 h-6 rounded-full bg-blue-600 flex items-center justify-center text-[11px] font-bold text-white flex-shrink-0">
                {day}
              </div>
              <h3 className="text-xs font-semibold text-gray-600 uppercase tracking-wider flex-1">
                Day {day}
              </h3>
              <span className="text-[11px] text-gray-400 mr-1">
                {byDay[day].length} stop{byDay[day].length !== 1 ? 's' : ''}
              </span>
              <button
                onClick={() => setConfirmDay(confirmDay === day ? null : day)}
                className={`p-1 rounded-md transition-all ${
                  confirmDay === day
                    ? 'bg-red-100 text-red-500'
                    : 'text-gray-300 hover:text-red-400 hover:bg-red-50 opacity-0 group-hover:opacity-100 focus:opacity-100 focus-visible:opacity-100'
                }`}
                aria-label={`Delete Day ${day}`}
                aria-expanded={confirmDay === day}
              >
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Delete confirmation */}
            {confirmDay === day && (
              <DeleteDayConfirm
                day={day}
                count={byDay[day].length}
                onConfirm={() => { onDeleteDay(day); setConfirmDay(null); }}
                onCancel={() => setConfirmDay(null)}
              />
            )}

            {/* Sortable item list for this day */}
            <SortableContext
              id={`day-${day}`}
              items={byDay[day].map(i => i.id)}
              strategy={verticalListSortingStrategy}
            >
              <ul aria-label={`Stops for Day ${day}`}>
                {byDay[day].map((item, idx) => (
                  <SortableItem
                    key={item.id}
                    item={item}
                    index={idx}
                    onRemove={onRemove}
                  />
                ))}
              </ul>
            </SortableContext>
          </section>
        ))}
      </div>

      {/* Floating drag preview */}
      <DragOverlay dropAnimation={{ duration: 180, easing: 'ease' }}>
        {activeItem ? (
          <ItineraryCard
            item={activeItem}
            index={0}
            overlay
          />
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
