import type { Metadata } from "next";
import { CalibrationPage } from "./calibration";

export const metadata: Metadata = { title: "Calibration" };

export default function Page() {
  return <CalibrationPage />;
}
