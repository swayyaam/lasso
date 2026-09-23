/**
 * The app's icon vocabulary.
 *
 * Every icon the interface uses is named here and imported from here, rather
 * than reached for directly out of lucide wherever it is needed. That is what
 * stops the same action picking a different glyph in two places — a queue row
 * cancelling with an X while a composer cancels with a Ban — and it makes the
 * whole vocabulary reviewable in one screen.
 *
 * Names describe the action, not the picture: `Retry`, not `RotateCcw`. When
 * the right glyph for an action changes, only this file does.
 *
 * lucide draws on a 24px grid with a 2px stroke. At the 14px the interface
 * renders them, stroke 2 reads as heavy against the system's 400/500 type
 * weights, so callers pass strokeWidth={1.75}. DESIGN-webflow.md's weight
 * ceiling is 600 and its voice is restrained; a 2px icon beside a 500-weight
 * label is louder than the label.
 */
export {
  // Resolving and downloading
  Search as Resolve,
  Download,
  Link as LinkIcon,

  // Queue actions
  Pause,
  Play as Resume,
  X as Cancel,
  RotateCcw as Retry,
  Trash2 as Clear,

  // Finished downloads
  FolderOpen as RevealInFinder,
  ExternalLink as OpenFile,
  Copy,

  // What a download is
  Video,
  Music as Audio,
  ListVideo as Playlist,

  // Navigation and panes
  ListVideo as Queue,
  History,
  ArrowLeft as Back,
  Search as Find,
  Settings,
  SlidersHorizontal as Advanced,
  Terminal as Command,
  Bookmark as Preset,

  // Diagnostics
  Stethoscope as Doctor,
  Wrench as Fix,
  RefreshCw as Recheck,
  CircleCheck as Ok,
  TriangleAlert as Warn,
  CircleX as Fail,
  Info,

  // Structure
  ChevronDown,
  ChevronRight,
  Folder,
  HardDrive,
  Cookie,
} from "lucide-react";
