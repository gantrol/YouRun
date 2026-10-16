import { useState } from "react";
import { useAppearance } from "./appearance";
import { launcherSkins } from "./design";
import type { ProjectDetails } from "./project-details";

export function ProjectLogo({
  name,
  logo,
  logos,
  scannedAt,
}: {
  name: string;
  logo?: ProjectDetails["logo"];
  logos?: ProjectDetails["logos"];
  scannedAt?: string;
}) {
  const { skin } = useAppearance();
  const [failures, setFailures] = useState<{ scan?: string; sources: string[] }>({
    sources: [],
  });
  const failed = failures.scan === scannedAt ? failures.sources : [];
  const images = (logo ? [logo] : (logos ?? []).slice(0, 4)).filter(
    (image) => !failed.includes(image.data),
  );
  return (
    <span
      aria-hidden="true"
      className={`grid size-8 shrink-0 place-items-center p-0.5 text-xs ${launcherSkins[skin].badge}`}
    >
      {images.length > 0 ? (
        <span
          className={`grid size-full gap-px ${images.length > 1 ? "grid-cols-2" : "grid-cols-1"}`}
        >
          {images.map((image, index) => (
            <img
              key={`${image.source}:${index}`}
              src={image.data}
              alt=""
              draggable={false}
              decoding="async"
              className="size-full min-h-0 min-w-0 object-contain"
              onError={() =>
                setFailures((previous) => ({
                  scan: scannedAt,
                  sources: [
                    ...(previous.scan === scannedAt ? previous.sources : []),
                    image.data,
                  ],
                }))
              }
            />
          ))}
        </span>
      ) : (
        Array.from(name.trim())[0]?.toLocaleUpperCase()
      )}
    </span>
  );
}
