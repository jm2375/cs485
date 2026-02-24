export type Role = 'Owner' | 'Editor' | 'Viewer';
export type POICategory = 'restaurant' | 'landmark' | 'hotel' | 'attraction';
export type PanelTab = 'collaborators' | 'search' | 'itinerary';

export interface Collaborator {
  id: string;
  name: string;
  email: string;
  role: Role;
  color: string;
  avatarUrl?: string;
}

export interface POI {
  id: string;
  name: string;
  category: POICategory;
  subcategory: string;
  address: string;
  rating: number;
  reviewCount: number;
  description: string;
  imageUrl: string;
  lat: number;
  lng: number;
  priceLevel: 1 | 2 | 3 | 4;
}

export interface ItineraryItem {
  id: string;
  poi: POI;
  addedBy: string;
  day: number;
  notes?: string;
}

export interface Trip {
  id: string;
  name: string;
  destination: string;
  shareLink: string;
  collaborators: Collaborator[];
}

export interface Toast {
  id: string;
  message: string;
  type: 'success' | 'info' | 'error';
}
