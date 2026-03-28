import React from 'react'
import { motion, AnimatePresence } from 'framer-motion'

export default function ConfirmModal({ isOpen, title, message, onConfirm, onCancel, danger = false }) {
  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          className="modal-overlay"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={onCancel}
        >
          <motion.div
            className="modal-card"
            initial={{ opacity: 0, scale: 0.95, y: 20 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: 20 }}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
            onClick={e => e.stopPropagation()}
          >
            <h3 className="modal-title">{title}</h3>
            <p className="modal-message">{message}</p>
            <div className="modal-actions">
              <button className="sel-pill neutral modal-pill" onClick={onCancel}>Cancel</button>
              <button className={`sel-pill ${danger ? 'red' : 'cyan'} modal-pill`} onClick={onConfirm}>
                {title}
              </button>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
