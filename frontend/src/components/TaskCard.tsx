import { PixelIcon } from "./PixelIcon";
import type { Task } from "../lib/api";

/*
 * One task, drawn as a plate and a caption. Shared rather than duplicated
 * because the same card is the grid on the home screen and the suggestions
 * under a finished job, and two copies would drift.
 */

export function TaskCard({
  task,
  index,
  onPick,
}: {
  task: Task;
  index: number;
  onPick: () => void;
}) {
  return (
    <button type="button" className="task-card" onClick={onPick}>
      <span className="task-plate">
        <span className="task-icon">
          <PixelIcon name={task.icon} size={40} />
        </span>
        {/* Honest before the click, not after. */}
        {!task.available && task.downloadMB > 0 ? (
          <span className="badge badge-warn">+{task.downloadMB} MB</span>
        ) : !task.available ? (
          <span className="badge badge-warn">Needs setup</span>
        ) : (
          <span className="task-index">{String(index).padStart(3, "0")}</span>
        )}
      </span>
      <span className="task-body">
        <span className="task-name">{task.name}</span>
        <span className="task-desc">{task.description}</span>
      </span>
    </button>
  );
}
