import type { SVGProps } from "react";
import paths from "../../resources/branding/yourun-rabbit-paths.json";
import "./yourun-logo.css";

type YouRunLogoProps = Omit<
  SVGProps<SVGSVGElement>,
  "children" | "width" | "height"
> & {
  size?: number;
  compact?: boolean;
  monochrome?: boolean;
  label?: string;
};

/** Shares vector paths with native branding and follows the active data-skin. */
export function YouRunLogo({
  size = 24,
  compact = size < 32,
  monochrome = false,
  label,
  className,
  ...props
}: YouRunLogoProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 128 128"
      fill="none"
      role={label ? "img" : undefined}
      aria-hidden={label ? undefined : true}
      aria-label={label}
      focusable="false"
      className={`yourun-logo${className ? ` ${className}` : ""}`}
      {...props}
    >
      <RabbitMark compact={compact} monochrome={monochrome} />
    </svg>
  );
}

function RabbitMark({ compact, monochrome }: { compact: boolean; monochrome: boolean }) {
  const eyeRadius = compact ? 4.3 : 3.7;
  const eye = `M${100 - eyeRadius} 64a${eyeRadius} ${eyeRadius} 0 1 0 ${eyeRadius * 2} 0a${eyeRadius} ${eyeRadius} 0 1 0 ${-eyeRadius * 2} 0Z`;
  const ink = "var(--yourun-logo-ink, var(--foreground, #24231f))";

  return (
    <>
      <g className="yourun-rabbit-classic" fillRule="evenodd">
        <path d={`${paths.silhouette}${eye}`} />
      </g>
      <g className="yourun-rabbit-comic">
        <path
          d={monochrome ? `${paths.comic}${eye}` : paths.comic}
          fillRule={monochrome ? "evenodd" : undefined}
          stroke={monochrome ? undefined : ink}
          strokeWidth={compact ? 4.8 : 4}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {!monochrome && <circle
          cx="100"
          cy="64"
          r={eyeRadius}
          fill="var(--yourun-logo-eye, var(--yourun-logo-ink, var(--foreground, #24231f)))"
        />}
      </g>
      <g className="yourun-rabbit-graphic" fillRule="evenodd">
        <path d={`${paths.silhouette}${paths.ears}${eye}`} />
      </g>
      <g className="yourun-rabbit-pixel" fillRule="evenodd" shapeRendering="crispEdges">
        <path d={paths.pixel} />
      </g>
    </>
  );
}
