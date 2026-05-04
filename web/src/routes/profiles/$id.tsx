import { useParams } from 'react-router-dom'
import { MappingEditor } from './mapping-editor'

/**
 * Profile editor wrapper — mounts MappingEditor in either "new" mode
 * (route `/profiles/new`, no id param) or "edit" mode (route
 * `/profiles/:id`).
 *
 * UI-SPEC explicit UX-01 deviation: the editor is a single-page surface,
 * NOT a dialog — too many fields and a live preview to fit modally.
 */
export default function ProfileEditorRoute() {
  const params = useParams<{ id?: string }>()

  // 'new' mode: route is /profiles/new (no :id) OR id literally === "new".
  if (!params.id || params.id === 'new') {
    return <MappingEditor mode="new" />
  }

  return <MappingEditor mode="edit" profileId={params.id} />
}
